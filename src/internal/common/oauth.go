// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED “AS IS” AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package common

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/pkce"
	"golang.org/x/crypto/bcrypt"
)

var OAuthSupportedScopes = []string{
	"profile:read",
	"repo:read",
	"repo:write",
	"tracker:read",
	"tracker:write",
	"ticket:read",
	"ticket:write",
	"notification:read",
	"notification:write",
}

var oauthSupportedScopeSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(OAuthSupportedScopes))
	for _, scope := range OAuthSupportedScopes {
		m[scope] = struct{}{}
	}
	return m
}()

type OAuthClientRecord struct {
	ClientID      string
	Name          string
	Description   string
	RedirectURIs  []string
	AllowedScopes []string
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type OAuthClientSecretRecord struct {
	OAuthClientRecord
	ClientSecret string
}

type OAuthConsentGrantRecord struct {
	GrantID         string
	ClientID        string
	ClientName      string
	UserID          int64
	RequestedScopes []string
	GrantedScopes   []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	RevokedAt       *time.Time
}

type OAuthUserProfile struct {
	UserID    int64
	Username  string
	ActorID   string
	MainKeyID string
}

type oauthStoredSession struct {
	Subject   string                 `json:"subject"`
	Username  string                 `json:"username"`
	ExpiresAt map[string]time.Time   `json:"expires_at"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

type oauthStoredRequest struct {
	Kind              string             `json:"kind"`
	ID                string             `json:"id"`
	RequestedAt       time.Time          `json:"requested_at"`
	ClientID          string             `json:"client_id"`
	RequestedScopes   []string           `json:"requested_scopes"`
	GrantedScopes     []string           `json:"granted_scopes"`
	RequestedAudience []string           `json:"requested_audience"`
	GrantedAudience   []string           `json:"granted_audience"`
	Form              url.Values         `json:"form"`
	Session           oauthStoredSession `json:"session"`
	RedirectURI       string             `json:"redirect_uri,omitempty"`
	State             string             `json:"state,omitempty"`
	ResponseTypes     []string           `json:"response_types,omitempty"`
	GrantTypes        []string           `json:"grant_types,omitempty"`
}

func IsOAuthScopeSupported(scope string) bool {
	_, ok := oauthSupportedScopeSet[strings.TrimSpace(scope)]
	return ok
}

func NormalizeOAuthScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrValidation)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(scopes))
	for _, raw := range scopes {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if !IsOAuthScopeSupported(s) {
			return nil, fmt.Errorf("%w: unsupported scope %q", ErrValidation, s)
		}
		if _, exists := seen[s]; exists {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrValidation)
	}
	sort.Strings(out)
	return out, nil
}

func ValidateOAuthRedirectURI(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%w: redirect uri is required", ErrValidation)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: invalid redirect uri: %v", ErrValidation, err)
	}
	if !u.IsAbs() || strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("%w: redirect uri must be absolute", ErrValidation)
	}
	if u.Fragment != "" {
		return fmt.Errorf("%w: redirect uri cannot contain fragment", ErrValidation)
	}
	hostname := strings.ToLower(strings.TrimSpace(u.Hostname()))
	isLoopback := hostname == "localhost"
	if ip := net.ParseIP(hostname); ip != nil && ip.IsLoopback() {
		isLoopback = true
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme == "https" {
		return nil
	}
	if scheme != "http" || !isLoopback {
		return fmt.Errorf("%w: redirect uri must use https unless loopback localhost", ErrValidation)
	}
	if u.Port() == "" {
		return fmt.Errorf("%w: localhost redirect uri must include explicit port", ErrValidation)
	}
	return nil
}

func normalizeRedirectURIs(uris []string) ([]string, error) {
	if len(uris) == 0 {
		return nil, fmt.Errorf("%w: at least one redirect uri is required", ErrValidation)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(uris))
	for _, raw := range uris {
		u := strings.TrimSpace(raw)
		if u == "" {
			continue
		}
		if err := ValidateOAuthRedirectURI(u); err != nil {
			return nil, err
		}
		if _, exists := seen[u]; exists {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one redirect uri is required", ErrValidation)
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) CreateOAuthClient(ctx context.Context, name, description string, redirectURIs, allowedScopes []string) (OAuthClientSecretRecord, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return OAuthClientSecretRecord{}, fmt.Errorf("%w: name is required", ErrValidation)
	}
	redirectURIs, err := normalizeRedirectURIs(redirectURIs)
	if err != nil {
		return OAuthClientSecretRecord{}, err
	}
	allowedScopes, err = NormalizeOAuthScopes(allowedScopes)
	if err != nil {
		return OAuthClientSecretRecord{}, err
	}

	clientID, clientSecret, err := generateOAuthClientCredentials()
	if err != nil {
		return OAuthClientSecretRecord{}, err
	}
	secretHash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		return OAuthClientSecretRecord{}, err
	}

	redirectRaw, err := json.Marshal(redirectURIs)
	if err != nil {
		return OAuthClientSecretRecord{}, err
	}
	scopesRaw, err := json.Marshal(allowedScopes)
	if err != nil {
		return OAuthClientSecretRecord{}, err
	}

	err = withTx(s.db, ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO oauth_client (client_id, name, description, redirect_uris, allowed_scopes, enabled, created_at, updated_at)
		         VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, TRUE, now(), now())`,
			clientID, name, description, string(redirectRaw), string(scopesRaw),
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO oauth_client_secret (client_id, secret_hash, created_at, updated_at)
		         VALUES ($1, $2, now(), now())`,
			clientID, string(secretHash),
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return OAuthClientSecretRecord{}, err
	}

	now := time.Now().UTC()
	return OAuthClientSecretRecord{
		OAuthClientRecord: OAuthClientRecord{
			ClientID:      clientID,
			Name:          name,
			Description:   description,
			RedirectURIs:  redirectURIs,
			AllowedScopes: allowedScopes,
			Enabled:       true,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		ClientSecret: clientSecret,
	}, nil
}

func (s *Store) RotateOAuthClientSecret(ctx context.Context, clientID string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return "", fmt.Errorf("%w: client id is required", ErrValidation)
	}

	clientSecret, err := randomOAuthToken(48)
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE oauth_client_secret
		    SET secret_hash = $2,
		        updated_at = now()
		  WHERE client_id = $1`,
		clientID,
		string(hash),
	)
	if err != nil {
		return "", err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows == 0 {
		return "", fmt.Errorf("%w: oauth client not found", ErrValidation)
	}
	return clientSecret, nil
}

func (s *Store) SetOAuthClientEnabled(ctx context.Context, clientID string, enabled bool) error {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return fmt.Errorf("%w: client id is required", ErrValidation)
	}
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE oauth_client
			    SET enabled = $2,
			        updated_at = now()
			  WHERE client_id = $1`,
			clientID,
			enabled,
		)
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return fmt.Errorf("%w: oauth client not found", ErrValidation)
		}
		if !enabled {
			if _, err := tx.ExecContext(ctx,
				`UPDATE oauth_consent_grant
				    SET revoked_at = COALESCE(revoked_at, now()),
				        updated_at = now()
				  WHERE client_id = $1`,
				clientID,
			); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE oauth_authorization_code
				    SET active = FALSE,
				        consumed_at = COALESCE(consumed_at, now())
				  WHERE client_id = $1`,
				clientID,
			); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE oauth_access_token
				    SET revoked_at = COALESCE(revoked_at, now())
				  WHERE client_id = $1`,
				clientID,
			); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE oauth_refresh_token
				    SET active = FALSE,
				        revoked_at = COALESCE(revoked_at, now()),
				        used_at = COALESCE(used_at, now())
				  WHERE client_id = $1`,
				clientID,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ListOAuthClients(ctx context.Context) ([]OAuthClientRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT client_id,
		        name,
		        description,
		        redirect_uris,
		        allowed_scopes,
		        enabled,
		        created_at,
		        updated_at
		   FROM oauth_client
		  ORDER BY created_at, client_id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []OAuthClientRecord{}
	for rows.Next() {
		var (
			rec         OAuthClientRecord
			redirectRaw []byte
			scopesRaw   []byte
		)
		if err := rows.Scan(
			&rec.ClientID,
			&rec.Name,
			&rec.Description,
			&redirectRaw,
			&scopesRaw,
			&rec.Enabled,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rec.RedirectURIs, err = decodeStringSliceJSON(redirectRaw)
		if err != nil {
			return nil, err
		}
		rec.AllowedScopes, err = decodeStringSliceJSON(scopesRaw)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetOAuthClient(ctx context.Context, clientID string) (OAuthClientRecord, bool, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return OAuthClientRecord{}, false, nil
	}
	var (
		rec         OAuthClientRecord
		redirectRaw []byte
		scopesRaw   []byte
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT client_id,
		        name,
		        description,
		        redirect_uris,
		        allowed_scopes,
		        enabled,
		        created_at,
		        updated_at
		   FROM oauth_client
		  WHERE client_id = $1`,
		clientID,
	).Scan(
		&rec.ClientID,
		&rec.Name,
		&rec.Description,
		&redirectRaw,
		&scopesRaw,
		&rec.Enabled,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OAuthClientRecord{}, false, nil
		}
		return OAuthClientRecord{}, false, err
	}
	rec.RedirectURIs, err = decodeStringSliceJSON(redirectRaw)
	if err != nil {
		return OAuthClientRecord{}, false, err
	}
	rec.AllowedScopes, err = decodeStringSliceJSON(scopesRaw)
	if err != nil {
		return OAuthClientRecord{}, false, err
	}
	return rec, true, nil
}

func generateOAuthClientCredentials() (clientID string, clientSecret string, err error) {
	idPart, err := randomOAuthToken(18)
	if err != nil {
		return "", "", err
	}
	secretPart, err := randomOAuthToken(48)
	if err != nil {
		return "", "", err
	}
	return "bvapp_" + idPart, secretPart, nil
}

func randomOAuthToken(rawBytes int) (string, error) {
	buf := make([]byte, rawBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func decodeStringSliceJSON(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func encodeStringSliceJSON(values []string) (string, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func makeStoredSession(session fosite.Session) oauthStoredSession {
	out := oauthStoredSession{
		ExpiresAt: map[string]time.Time{},
	}
	if session == nil {
		return out
	}
	out.Subject = strings.TrimSpace(session.GetSubject())
	out.Username = strings.TrimSpace(session.GetUsername())
	for _, tokenType := range []fosite.TokenType{fosite.AccessToken, fosite.RefreshToken, fosite.AuthorizeCode} {
		exp := session.GetExpiresAt(tokenType)
		if !exp.IsZero() {
			out.ExpiresAt[string(tokenType)] = exp.UTC()
		}
	}
	if ds, ok := session.(*fosite.DefaultSession); ok && ds != nil && len(ds.Extra) > 0 {
		out.Extra = map[string]interface{}{}
		for k, v := range ds.Extra {
			out.Extra[k] = v
		}
	}
	return out
}

func makeDefaultSession(stored oauthStoredSession) *fosite.DefaultSession {
	ds := &fosite.DefaultSession{
		Subject:  stored.Subject,
		Username: stored.Username,
		Extra:    map[string]interface{}{},
	}
	for k, v := range stored.ExpiresAt {
		tt := fosite.TokenType(k)
		ds.SetExpiresAt(tt, v)
	}
	for k, v := range stored.Extra {
		ds.Extra[k] = v
	}
	return ds
}

func snapshotRequester(req fosite.Requester) (oauthStoredRequest, error) {
	if req == nil || req.GetClient() == nil {
		return oauthStoredRequest{}, errors.New("request client is required")
	}
	snap := oauthStoredRequest{
		Kind:              "request",
		ID:                req.GetID(),
		RequestedAt:       req.GetRequestedAt().UTC(),
		ClientID:          req.GetClient().GetID(),
		RequestedScopes:   append([]string(nil), req.GetRequestedScopes()...),
		GrantedScopes:     append([]string(nil), req.GetGrantedScopes()...),
		RequestedAudience: append([]string(nil), req.GetRequestedAudience()...),
		GrantedAudience:   append([]string(nil), req.GetGrantedAudience()...),
		Form:              cloneURLValues(req.GetRequestForm()),
		Session:           makeStoredSession(req.GetSession()),
	}

	if ar, ok := req.(fosite.AuthorizeRequester); ok {
		snap.Kind = "authorize"
		if ar.GetRedirectURI() != nil {
			snap.RedirectURI = ar.GetRedirectURI().String()
		}
		snap.State = ar.GetState()
		snap.ResponseTypes = append([]string(nil), ar.GetResponseTypes()...)
		return snap, nil
	}
	if tr, ok := req.(fosite.AccessRequester); ok {
		snap.Kind = "access"
		snap.GrantTypes = append([]string(nil), tr.GetGrantTypes()...)
		return snap, nil
	}
	return snap, nil
}

func cloneURLValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for k, vals := range in {
		cp := make([]string, len(vals))
		copy(cp, vals)
		out[k] = cp
	}
	return out
}

func (s *Store) requesterFromSnapshot(ctx context.Context, snap oauthStoredRequest, allowDisabled bool) (fosite.Requester, error) {
	client, err := s.getOAuthClientForFosite(ctx, snap.ClientID, allowDisabled)
	if err != nil {
		return nil, err
	}
	session := makeDefaultSession(snap.Session)

	switch snap.Kind {
	case "request":
		req := fosite.NewRequest()
		req.SetID(snap.ID)
		req.RequestedAt = snap.RequestedAt
		req.Client = client
		req.SetRequestedScopes(fosite.Arguments(snap.RequestedScopes))
		req.SetRequestedAudience(fosite.Arguments(snap.RequestedAudience))
		for _, scope := range snap.GrantedScopes {
			req.GrantScope(scope)
		}
		for _, aud := range snap.GrantedAudience {
			req.GrantAudience(aud)
		}
		req.Form = cloneURLValues(snap.Form)
		req.Session = session
		return req, nil
	case "authorize":
		ar := fosite.NewAuthorizeRequest()
		ar.SetID(snap.ID)
		ar.RequestedAt = snap.RequestedAt
		ar.Client = client
		ar.SetRequestedScopes(fosite.Arguments(snap.RequestedScopes))
		ar.SetRequestedAudience(fosite.Arguments(snap.RequestedAudience))
		for _, scope := range snap.GrantedScopes {
			ar.GrantScope(scope)
		}
		for _, aud := range snap.GrantedAudience {
			ar.GrantAudience(aud)
		}
		ar.Form = cloneURLValues(snap.Form)
		ar.Session = session
		ar.ResponseTypes = append(ar.ResponseTypes[:0], snap.ResponseTypes...)
		ar.State = snap.State
		if snap.RedirectURI != "" {
			u, err := url.Parse(snap.RedirectURI)
			if err != nil {
				return nil, err
			}
			ar.RedirectURI = u
		}
		return ar, nil
	case "access":
		tr := fosite.NewAccessRequest(session)
		tr.SetID(snap.ID)
		tr.RequestedAt = snap.RequestedAt
		tr.Client = client
		tr.SetRequestedScopes(fosite.Arguments(snap.RequestedScopes))
		tr.SetRequestedAudience(fosite.Arguments(snap.RequestedAudience))
		for _, scope := range snap.GrantedScopes {
			tr.GrantScope(scope)
		}
		for _, aud := range snap.GrantedAudience {
			tr.GrantAudience(aud)
		}
		tr.Form = cloneURLValues(snap.Form)
		tr.GrantTypes = append(tr.GrantTypes[:0], snap.GrantTypes...)
		return tr, nil
	default:
		return nil, errors.New("unknown requester kind")
	}
}

func userIDFromRequester(req fosite.Requester) (int64, bool) {
	session := req.GetSession()
	if session == nil {
		return 0, false
	}
	sub := strings.TrimSpace(session.GetSubject())
	if sub == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func grantIDFromRequester(req fosite.Requester) string {
	session := req.GetSession()
	ds, ok := session.(*fosite.DefaultSession)
	if !ok || ds == nil || ds.Extra == nil {
		return ""
	}
	raw, ok := ds.Extra["grant_id"]
	if !ok {
		return ""
	}
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func refreshFamilyIDFromRequester(req fosite.Requester) string {
	session := req.GetSession()
	ds, ok := session.(*fosite.DefaultSession)
	if !ok || ds == nil || ds.Extra == nil {
		return ""
	}
	raw, ok := ds.Extra["refresh_family_id"]
	if !ok {
		return ""
	}
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func defaultTokenExpiry(req fosite.Requester, tokenType fosite.TokenType, fallback time.Duration) time.Time {
	if req.GetSession() != nil {
		exp := req.GetSession().GetExpiresAt(tokenType)
		if !exp.IsZero() {
			return exp.UTC()
		}
	}
	return req.GetRequestedAt().UTC().Add(fallback)
}

func (s *Store) getOAuthClientForFosite(ctx context.Context, id string, allowDisabled bool) (*fosite.DefaultClient, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fosite.ErrNotFound
	}
	var (
		secretHash  string
		redirectRaw []byte
		scopesRaw   []byte
		enabled     bool
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT c.enabled, c.redirect_uris, c.allowed_scopes, cs.secret_hash
		   FROM oauth_client c
		   JOIN oauth_client_secret cs ON cs.client_id = c.client_id
		  WHERE c.client_id = $1`,
		id,
	).Scan(&enabled, &redirectRaw, &scopesRaw, &secretHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	if !allowDisabled && !enabled {
		return nil, fosite.ErrNotFound
	}
	redirectURIs, err := decodeStringSliceJSON(redirectRaw)
	if err != nil {
		return nil, err
	}
	scopes, err := decodeStringSliceJSON(scopesRaw)
	if err != nil {
		return nil, err
	}
	return &fosite.DefaultClient{
		ID:            id,
		Secret:        []byte(secretHash),
		RedirectURIs:  redirectURIs,
		GrantTypes:    []string{string(fosite.GrantTypeAuthorizationCode), string(fosite.GrantTypeRefreshToken)},
		ResponseTypes: []string{"code"},
		Scopes:        scopes,
		Public:        false,
	}, nil
}

func (s *Store) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	return s.getOAuthClientForFosite(ctx, id, false)
}

func (s *Store) ClientAssertionJWTValid(ctx context.Context, jti string) error {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return fosite.ErrNotFound
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM oauth_client_assertion_jti WHERE expires_at <= now()`); err != nil {
		return err
	}
	var known string
	err := s.db.QueryRowContext(ctx,
		`SELECT jti FROM oauth_client_assertion_jti WHERE jti = $1`,
		jti,
	).Scan(&known)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	return fosite.ErrJTIKnown
}

func (s *Store) SetClientAssertionJWT(ctx context.Context, jti string, exp time.Time) error {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return fosite.ErrNotFound
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM oauth_client_assertion_jti WHERE expires_at <= now()`); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO oauth_client_assertion_jti (jti, expires_at)
		 VALUES ($1, $2)
		 ON CONFLICT (jti) DO NOTHING`,
		jti,
		exp.UTC(),
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fosite.ErrJTIKnown
	}
	return nil
}

func (s *Store) CreateAuthorizeCodeSession(ctx context.Context, code string, request fosite.Requester) error {
	snap, err := snapshotRequester(request)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	clientID := request.GetClient().GetID()
	var userID sql.NullInt64
	if uid, ok := userIDFromRequester(request); ok {
		userID = sql.NullInt64{Int64: uid, Valid: true}
	}
	grantID := strings.TrimSpace(grantIDFromRequester(request))
	var grantIDNull sql.NullString
	if grantID != "" {
		grantIDNull = sql.NullString{String: grantID, Valid: true}
	}
	expiresAt := defaultTokenExpiry(request, fosite.AuthorizeCode, 15*time.Minute)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO oauth_authorization_code (signature, request_id, client_id, user_id, grant_id, request_json, active, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, TRUE, now(), $7)
		 ON CONFLICT (signature)
		 DO UPDATE SET
		   request_id = EXCLUDED.request_id,
		   client_id = EXCLUDED.client_id,
		   user_id = EXCLUDED.user_id,
		   grant_id = EXCLUDED.grant_id,
		   request_json = EXCLUDED.request_json,
		   active = TRUE,
		   consumed_at = NULL,
		   expires_at = EXCLUDED.expires_at`,
		code,
		request.GetID(),
		clientID,
		userID,
		grantIDNull,
		string(raw),
		expiresAt,
	)
	return err
}

func (s *Store) GetAuthorizeCodeSession(ctx context.Context, code string, session fosite.Session) (fosite.Requester, error) {
	var (
		requestRaw []byte
		active     bool
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT request_json, active
		   FROM oauth_authorization_code
		  WHERE signature = $1`,
		code,
	).Scan(&requestRaw, &active)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	var snap oauthStoredRequest
	if err := json.Unmarshal(requestRaw, &snap); err != nil {
		return nil, err
	}
	req, err := s.requesterFromSnapshot(ctx, snap, true)
	if err != nil {
		return nil, err
	}
	if !active {
		return req, fosite.ErrInvalidatedAuthorizeCode
	}
	return req, nil
}

func (s *Store) InvalidateAuthorizeCodeSession(ctx context.Context, code string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE oauth_authorization_code
		    SET active = FALSE,
		        consumed_at = now()
		  WHERE signature = $1`,
		code,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fosite.ErrNotFound
	}
	return nil
}

func (s *Store) CreatePKCERequestSession(ctx context.Context, signature string, requester fosite.Requester) error {
	snap, err := snapshotRequester(requester)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO oauth_pkce_request (signature, request_json, created_at)
		 VALUES ($1, $2::jsonb, now())
		 ON CONFLICT (signature)
		 DO UPDATE SET request_json = EXCLUDED.request_json`,
		signature,
		string(raw),
	)
	return err
}

func (s *Store) GetPKCERequestSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	var requestRaw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT request_json FROM oauth_pkce_request WHERE signature = $1`,
		signature,
	).Scan(&requestRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	var snap oauthStoredRequest
	if err := json.Unmarshal(requestRaw, &snap); err != nil {
		return nil, err
	}
	return s.requesterFromSnapshot(ctx, snap, true)
}

func (s *Store) DeletePKCERequestSession(ctx context.Context, signature string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_pkce_request WHERE signature = $1`, signature)
	return err
}

func (s *Store) CreateAccessTokenSession(ctx context.Context, signature string, requester fosite.Requester) error {
	snap, err := snapshotRequester(requester)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	clientID := requester.GetClient().GetID()
	var userID sql.NullInt64
	if uid, ok := userIDFromRequester(requester); ok {
		userID = sql.NullInt64{Int64: uid, Valid: true}
	}
	var grantID sql.NullString
	if g := grantIDFromRequester(requester); g != "" {
		grantID = sql.NullString{String: g, Valid: true}
	}
	expiresAt := defaultTokenExpiry(requester, fosite.AccessToken, time.Hour)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO oauth_access_token (signature, request_id, client_id, user_id, grant_id, request_json, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, now(), $7)
		 ON CONFLICT (signature)
		 DO UPDATE SET
		   request_id = EXCLUDED.request_id,
		   client_id = EXCLUDED.client_id,
		   user_id = EXCLUDED.user_id,
		   grant_id = EXCLUDED.grant_id,
		   request_json = EXCLUDED.request_json,
		   expires_at = EXCLUDED.expires_at,
		   revoked_at = NULL`,
		signature,
		requester.GetID(),
		clientID,
		userID,
		grantID,
		string(raw),
		expiresAt,
	)
	return err
}

func (s *Store) GetAccessTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	var requestRaw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT request_json
		   FROM oauth_access_token
		  WHERE signature = $1
		    AND revoked_at IS NULL`,
		signature,
	).Scan(&requestRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	var snap oauthStoredRequest
	if err := json.Unmarshal(requestRaw, &snap); err != nil {
		return nil, err
	}
	return s.requesterFromSnapshot(ctx, snap, true)
}

func (s *Store) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE oauth_access_token
		    SET revoked_at = COALESCE(revoked_at, now())
		  WHERE signature = $1`,
		signature,
	)
	return err
}

func (s *Store) CreateRefreshTokenSession(ctx context.Context, signature string, accessSignature string, requester fosite.Requester) error {
	snap, err := snapshotRequester(requester)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	clientID := requester.GetClient().GetID()
	var userID sql.NullInt64
	if uid, ok := userIDFromRequester(requester); ok {
		userID = sql.NullInt64{Int64: uid, Valid: true}
	}
	var grantID sql.NullString
	if g := grantIDFromRequester(requester); g != "" {
		grantID = sql.NullString{String: g, Valid: true}
	}
	familyID := refreshFamilyIDFromRequester(requester)
	if familyID == "" {
		familyID = requester.GetID()
	}
	expiresAt := defaultTokenExpiry(requester, fosite.RefreshToken, 30*24*time.Hour)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO oauth_refresh_token (signature, request_id, access_token_signature, family_id, parent_signature, client_id, user_id, grant_id, request_json, active, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, NULL, $5, $6, $7, $8::jsonb, TRUE, now(), $9)
		 ON CONFLICT (signature)
		 DO UPDATE SET
		   request_id = EXCLUDED.request_id,
		   access_token_signature = EXCLUDED.access_token_signature,
		   family_id = EXCLUDED.family_id,
		   client_id = EXCLUDED.client_id,
		   user_id = EXCLUDED.user_id,
		   grant_id = EXCLUDED.grant_id,
		   request_json = EXCLUDED.request_json,
		   active = TRUE,
		   used_at = NULL,
		   revoked_at = NULL,
		   expires_at = EXCLUDED.expires_at`,
		signature,
		requester.GetID(),
		accessSignature,
		familyID,
		clientID,
		userID,
		grantID,
		string(raw),
		expiresAt,
	)
	return err
}

func (s *Store) GetRefreshTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	var (
		requestRaw []byte
		active     bool
		revokedAt  sql.NullTime
		expiresAt  time.Time
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT request_json, active, revoked_at, expires_at
		   FROM oauth_refresh_token
		  WHERE signature = $1`,
		signature,
	).Scan(&requestRaw, &active, &revokedAt, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	var snap oauthStoredRequest
	if err := json.Unmarshal(requestRaw, &snap); err != nil {
		return nil, err
	}
	req, err := s.requesterFromSnapshot(ctx, snap, true)
	if err != nil {
		return nil, err
	}
	if revokedAt.Valid || !active || expiresAt.Before(time.Now().UTC()) {
		return req, fosite.ErrInactiveToken
	}
	return req, nil
}

func (s *Store) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE oauth_refresh_token
		    SET active = FALSE,
		        used_at = COALESCE(used_at, now()),
		        revoked_at = COALESCE(revoked_at, now())
		  WHERE signature = $1`,
		signature,
	)
	return err
}

func (s *Store) RotateRefreshToken(ctx context.Context, requestID string, refreshTokenSignature string) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		var (
			active   bool
			familyID string
		)
		err := tx.QueryRowContext(ctx,
			`SELECT active, family_id
			   FROM oauth_refresh_token
			  WHERE signature = $1`,
			refreshTokenSignature,
		).Scan(&active, &familyID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fosite.ErrNotFound
			}
			return err
		}

		if !active {
			if _, err := tx.ExecContext(ctx,
				`UPDATE oauth_refresh_token
				    SET active = FALSE,
				        revoked_at = COALESCE(revoked_at, now()),
				        used_at = COALESCE(used_at, now())
				  WHERE family_id = $1`,
				familyID,
			); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE oauth_access_token
				    SET revoked_at = COALESCE(revoked_at, now())
				  WHERE request_id = $1`,
				requestID,
			); err != nil {
				return err
			}
			return fosite.ErrInactiveToken
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE oauth_refresh_token
			    SET active = FALSE,
			        used_at = now()
			  WHERE signature = $1`,
			refreshTokenSignature,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE oauth_access_token
			    SET revoked_at = COALESCE(revoked_at, now())
			  WHERE request_id = $1`,
			requestID,
		); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) RevokeRefreshToken(ctx context.Context, requestID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE oauth_refresh_token
		    SET active = FALSE,
		        revoked_at = COALESCE(revoked_at, now()),
		        used_at = COALESCE(used_at, now())
		  WHERE request_id = $1`,
		requestID,
	)
	return err
}

func (s *Store) RevokeAccessToken(ctx context.Context, requestID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE oauth_access_token
		    SET revoked_at = COALESCE(revoked_at, now())
		  WHERE request_id = $1`,
		requestID,
	)
	return err
}

func (s *Store) UpsertOAuthConsentGrant(ctx context.Context, clientID string, userID int64, requestedScopes, grantedScopes []string) (OAuthConsentGrantRecord, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return OAuthConsentGrantRecord{}, fmt.Errorf("%w: client id is required", ErrValidation)
	}
	if userID <= 0 {
		return OAuthConsentGrantRecord{}, fmt.Errorf("%w: user id is required", ErrValidation)
	}
	requestedScopes, err := NormalizeOAuthScopes(requestedScopes)
	if err != nil {
		return OAuthConsentGrantRecord{}, err
	}
	if len(grantedScopes) == 0 {
		return OAuthConsentGrantRecord{}, fmt.Errorf("%w: at least one granted scope is required", ErrValidation)
	}
	grantedScopes, err = NormalizeOAuthScopes(grantedScopes)
	if err != nil {
		return OAuthConsentGrantRecord{}, err
	}
	if !isSubset(grantedScopes, requestedScopes) {
		return OAuthConsentGrantRecord{}, fmt.Errorf("%w: granted scopes must be subset of requested scopes", ErrValidation)
	}

	reqRaw, err := encodeStringSliceJSON(requestedScopes)
	if err != nil {
		return OAuthConsentGrantRecord{}, err
	}
	grantRaw, err := encodeStringSliceJSON(grantedScopes)
	if err != nil {
		return OAuthConsentGrantRecord{}, err
	}
	grantID := uuid.NewString()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO oauth_consent_grant (grant_id, client_id, user_id, requested_scopes, granted_scopes, created_at, updated_at, revoked_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, now(), now(), NULL)`,
		grantID,
		clientID,
		userID,
		reqRaw,
		grantRaw,
	); err != nil {
		return OAuthConsentGrantRecord{}, err
	}

	grants, err := s.ListOAuthConsentGrantsByUser(ctx, userID)
	if err != nil {
		return OAuthConsentGrantRecord{}, err
	}
	for _, grant := range grants {
		if grant.GrantID == grantID {
			return grant, nil
		}
	}
	return OAuthConsentGrantRecord{}, errors.New("oauth consent grant not found after insert")
}

func (s *Store) ListOAuthConsentGrantsByUser(ctx context.Context, userID int64) ([]OAuthConsentGrantRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT g.grant_id,
		        g.client_id,
		        c.name,
		        g.user_id,
		        g.requested_scopes,
		        g.granted_scopes,
		        g.created_at,
		        g.updated_at,
		        g.revoked_at
		   FROM oauth_consent_grant g
		   JOIN oauth_client c ON c.client_id = g.client_id
		  WHERE g.user_id = $1
		  ORDER BY g.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []OAuthConsentGrantRecord{}
	for rows.Next() {
		var (
			rec          OAuthConsentGrantRecord
			requestedRaw []byte
			grantedRaw   []byte
			revokedAt    sql.NullTime
		)
		if err := rows.Scan(
			&rec.GrantID,
			&rec.ClientID,
			&rec.ClientName,
			&rec.UserID,
			&requestedRaw,
			&grantedRaw,
			&rec.CreatedAt,
			&rec.UpdatedAt,
			&revokedAt,
		); err != nil {
			return nil, err
		}
		rec.RequestedScopes, err = decodeStringSliceJSON(requestedRaw)
		if err != nil {
			return nil, err
		}
		rec.GrantedScopes, err = decodeStringSliceJSON(grantedRaw)
		if err != nil {
			return nil, err
		}
		if revokedAt.Valid {
			t := revokedAt.Time.UTC()
			rec.RevokedAt = &t
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetLatestActiveOAuthConsentGrant(ctx context.Context, userID int64, clientID string) (OAuthConsentGrantRecord, bool, error) {
	clientID = strings.TrimSpace(clientID)
	if userID <= 0 || clientID == "" {
		return OAuthConsentGrantRecord{}, false, nil
	}
	var (
		rec          OAuthConsentGrantRecord
		requestedRaw []byte
		grantedRaw   []byte
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT g.grant_id,
		        g.client_id,
		        c.name,
		        g.user_id,
		        g.requested_scopes,
		        g.granted_scopes,
		        g.created_at,
		        g.updated_at
		   FROM oauth_consent_grant g
		   JOIN oauth_client c ON c.client_id = g.client_id
		  WHERE g.user_id = $1
		    AND g.client_id = $2
		    AND g.revoked_at IS NULL
		  ORDER BY g.created_at DESC
		  LIMIT 1`,
		userID,
		clientID,
	).Scan(
		&rec.GrantID,
		&rec.ClientID,
		&rec.ClientName,
		&rec.UserID,
		&requestedRaw,
		&grantedRaw,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OAuthConsentGrantRecord{}, false, nil
		}
		return OAuthConsentGrantRecord{}, false, err
	}
	rec.RequestedScopes, err = decodeStringSliceJSON(requestedRaw)
	if err != nil {
		return OAuthConsentGrantRecord{}, false, err
	}
	rec.GrantedScopes, err = decodeStringSliceJSON(grantedRaw)
	if err != nil {
		return OAuthConsentGrantRecord{}, false, err
	}
	return rec, true, nil
}

func (s *Store) RevokeOAuthConsentGrant(ctx context.Context, userID int64, grantID string) error {
	grantID = strings.TrimSpace(grantID)
	if grantID == "" {
		return fmt.Errorf("%w: grant id is required", ErrValidation)
	}
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE oauth_consent_grant
			    SET revoked_at = COALESCE(revoked_at, now()),
			        updated_at = now()
			  WHERE grant_id = $1
			    AND user_id = $2`,
			grantID,
			userID,
		)
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return fmt.Errorf("%w: oauth grant not found", ErrValidation)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE oauth_access_token
			    SET revoked_at = COALESCE(revoked_at, now())
			  WHERE grant_id = $1`,
			grantID,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE oauth_refresh_token
			    SET active = FALSE,
			        revoked_at = COALESCE(revoked_at, now()),
			        used_at = COALESCE(used_at, now())
			  WHERE grant_id = $1`,
			grantID,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE oauth_authorization_code
			    SET active = FALSE,
			        consumed_at = COALESCE(consumed_at, now())
			  WHERE grant_id = $1`,
			grantID,
		); err != nil {
			return err
		}
		return nil
	})
}

func isSubset(subset []string, superset []string) bool {
	if len(subset) == 0 {
		return true
	}
	m := map[string]struct{}{}
	for _, item := range superset {
		m[item] = struct{}{}
	}
	for _, item := range subset {
		if _, ok := m[item]; !ok {
			return false
		}
	}
	return true
}

func (s *Store) ListTickets(ctx context.Context) ([]Ticket, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id,
		        t.internal_id::text,
		        t.slug,
		        t.primary_key,
		        tr.slug,
		        r.slug,
		        COALESCE(t.body->>'summary', ''),
		        COALESCE(t.body->>'content', ''),
		        COALESCE(t.body->>'published', ''),
		        t.created_at,
		        COALESCE(t.priority, 0)
		   FROM ff_ticket t
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		   JOIN ff_repository r ON r.internal_id = t.repository_internal_id
		  ORDER BY t.created_at DESC, t.id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ticket{}
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(
			&t.ID,
			&t.InternalID,
			&t.Slug,
			&t.ActorID,
			&t.TrackerSlug,
			&t.RepositorySlug,
			&t.Summary,
			&t.Content,
			&t.Published,
			&t.CreatedAt,
			&t.Priority,
		); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListTicketsForTracker(ctx context.Context, trackerSlug string) ([]Ticket, error) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	if trackerSlug == "" {
		return []Ticket{}, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id,
		        t.internal_id::text,
		        t.slug,
		        t.primary_key,
		        tr.slug,
		        r.slug,
		        COALESCE(t.body->>'summary', ''),
		        COALESCE(t.body->>'content', ''),
		        COALESCE(t.body->>'published', ''),
		        t.created_at,
		        COALESCE(t.priority, 0)
		   FROM ff_ticket t
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		   JOIN ff_repository r ON r.internal_id = t.repository_internal_id
		  WHERE tr.slug = $1
		  ORDER BY t.created_at DESC, t.id DESC`,
		trackerSlug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ticket{}
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(
			&t.ID,
			&t.InternalID,
			&t.Slug,
			&t.ActorID,
			&t.TrackerSlug,
			&t.RepositorySlug,
			&t.Summary,
			&t.Content,
			&t.Published,
			&t.CreatedAt,
			&t.Priority,
		); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListTicketsForUser(ctx context.Context, userID int64) ([]Ticket, error) {
	if userID <= 0 {
		return []Ticket{}, nil
	}
	all, err := s.ListTickets(ctx)
	if err != nil {
		return nil, err
	}
	allowed := make([]Ticket, 0, len(all))
	for _, ticket := range all {
		canAccess, err := s.CanAccessRepository(ctx, ticket.RepositorySlug, userID)
		if err != nil {
			return nil, err
		}
		if canAccess {
			allowed = append(allowed, ticket)
		}
	}
	return allowed, nil
}

func (s *Store) ListTicketsForTrackerForUser(ctx context.Context, trackerSlug string, userID int64) ([]Ticket, error) {
	if userID <= 0 {
		return []Ticket{}, nil
	}
	all, err := s.ListTicketsForTracker(ctx, trackerSlug)
	if err != nil {
		return nil, err
	}
	allowed := make([]Ticket, 0, len(all))
	for _, ticket := range all {
		canAccess, err := s.CanAccessRepository(ctx, ticket.RepositorySlug, userID)
		if err != nil {
			return nil, err
		}
		if canAccess {
			allowed = append(allowed, ticket)
		}
	}
	return allowed, nil
}

func (s *Store) ListAssignedTicketsForUser(ctx context.Context, userID int64, options AssignedTicketListOptions) ([]AssignedTicket, error) {
	if userID <= 0 {
		return []AssignedTicket{}, nil
	}

	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	isAdmin, err := s.IsUserAdmin(ctx, userID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id,
		        t.primary_key,
		        tr.slug,
		        t.slug,
		        r.slug,
		        COALESCE(t.body->>'summary', ''),
		        COALESCE(t.body->>'content', ''),
		        t.created_at,
		        COALESCE(t.priority, 0),
		        EXISTS(
		            SELECT 1
		              FROM ff_ticket_comment c
		             WHERE c.ticket_internal_id = t.internal_id
		               AND c.created_by_user_id = $1
		        ) AS responded_by_me
		   FROM ff_ticket_assignee a
		   JOIN ff_ticket t ON t.internal_id = a.ticket_internal_id
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		   JOIN ff_repository r ON r.internal_id = t.repository_internal_id
		  WHERE a.user_id = $1
		    AND t.is_local = TRUE
		    AND (
		        $2::boolean = TRUE OR
		        EXISTS(
		            SELECT 1
		              FROM ff_repository_member m
		             WHERE m.repository_internal_id = t.repository_internal_id
		               AND m.user_id = $1
		        )
		    )
		    AND (
		        $3::boolean = FALSE OR
		        NOT EXISTS(
		            SELECT 1
		              FROM ff_ticket_comment c2
		             WHERE c2.ticket_internal_id = t.internal_id
		               AND c2.created_by_user_id = $1
		        )
		    )
		    AND (
		        $4::boolean = FALSE OR
		        NOT EXISTS(
		            SELECT 1
		              FROM ff_ticket_comment c3
		              JOIN as_note n3 ON n3.primary_key = c3.note_primary_key
		             WHERE c3.ticket_internal_id = t.internal_id
		               AND c3.created_by_user_id = $1
		               AND COALESCE(n3.body->>'borealValleyAgentCommentKind', '') = $5
		        )
		    )
		  ORDER BY t.created_at ASC, t.priority DESC, t.id ASC
		  LIMIT $6`,
		userID,
		isAdmin,
		options.UnrespondedOnly,
		options.AgentCompletionPendingOnly,
		AgentCommentKindCompletion,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tickets := []AssignedTicket{}
	for rows.Next() {
		var item AssignedTicket
		if err := rows.Scan(
			&item.ID,
			&item.ActorID,
			&item.TrackerSlug,
			&item.TicketSlug,
			&item.RepositorySlug,
			&item.Summary,
			&item.Content,
			&item.CreatedAt,
			&item.Priority,
			&item.RespondedByMe,
		); err != nil {
			return nil, err
		}
		tickets = append(tickets, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tickets, nil
}

func (s *Store) GetLocalRepositoryObjectBySlug(ctx context.Context, repoSlug string) (LocalObjectRecord, bool, error) {
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" {
		return LocalObjectRecord{}, false, nil
	}

	var record LocalObjectRecord
	var trackerActorID string
	err := s.db.QueryRowContext(ctx,
		`SELECT r.primary_key,
		        r.body,
		        COALESCE(t.primary_key, '')
		   FROM ff_repository r
		   LEFT JOIN ff_ticket_tracker t ON t.internal_id = r.ticket_tracker_internal_id
		  WHERE r.is_local = TRUE AND r.slug = $1`,
		repoSlug,
	).Scan(&record.PrimaryKey, &record.BodyJSON, &trackerActorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LocalObjectRecord{}, false, nil
		}
		return LocalObjectRecord{}, false, err
	}

	body, err := parseBody(record.BodyJSON)
	if err != nil {
		return LocalObjectRecord{}, false, err
	}
	if strings.TrimSpace(trackerActorID) == "" {
		delete(body, "ticketsTrackedBy")
	} else {
		body["ticketsTrackedBy"] = trackerActorID
	}
	record.BodyJSON, err = json.Marshal(body)
	if err != nil {
		return LocalObjectRecord{}, false, err
	}

	return record, true, nil
}

func (s *Store) GetLocalTicketTrackerObjectBySlug(ctx context.Context, trackerSlug string) (LocalObjectRecord, bool, error) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	if trackerSlug == "" {
		return LocalObjectRecord{}, false, nil
	}

	var (
		record     LocalObjectRecord
		internalID string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT internal_id::text, primary_key, body
		   FROM ff_ticket_tracker
		  WHERE is_local = TRUE AND slug = $1`,
		trackerSlug,
	).Scan(&internalID, &record.PrimaryKey, &record.BodyJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LocalObjectRecord{}, false, nil
		}
		return LocalObjectRecord{}, false, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT primary_key
		   FROM ff_repository
		  WHERE is_local = TRUE
		    AND ticket_tracker_internal_id = $1::uuid
		  ORDER BY slug`,
		internalID,
	)
	if err != nil {
		return LocalObjectRecord{}, false, err
	}
	defer rows.Close()

	trackedRepoActorIDs := []string{}
	for rows.Next() {
		var actorID string
		if err := rows.Scan(&actorID); err != nil {
			return LocalObjectRecord{}, false, err
		}
		trackedRepoActorIDs = append(trackedRepoActorIDs, actorID)
	}
	if err := rows.Err(); err != nil {
		return LocalObjectRecord{}, false, err
	}

	body, err := parseBody(record.BodyJSON)
	if err != nil {
		return LocalObjectRecord{}, false, err
	}
	body["tracksTicketsFor"] = trackedRepoActorIDs
	record.BodyJSON, err = json.Marshal(body)
	if err != nil {
		return LocalObjectRecord{}, false, err
	}

	return record, true, nil
}

func (s *Store) GetLocalTicketObjectBySlug(ctx context.Context, trackerSlug, ticketSlug string) (LocalTicketObjectRecord, bool, error) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	ticketSlug = strings.TrimSpace(ticketSlug)
	if trackerSlug == "" || ticketSlug == "" {
		return LocalTicketObjectRecord{}, false, nil
	}

	var (
		record           LocalTicketObjectRecord
		ticketInternalID string
		priority         int
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT t.internal_id::text,
		        t.primary_key,
		        t.body,
		        tr.slug,
		        t.slug,
		        r.slug,
		        COALESCE(t.priority, 0)
		   FROM ff_ticket t
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		   JOIN ff_repository r ON r.internal_id = t.repository_internal_id
		  WHERE t.is_local = TRUE
		    AND tr.slug = $1
		    AND t.slug = $2`,
		trackerSlug,
		ticketSlug,
	).Scan(
		&ticketInternalID,
		&record.PrimaryKey,
		&record.BodyJSON,
		&record.TrackerSlug,
		&record.TicketSlug,
		&record.RepositorySlug,
		&priority,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LocalTicketObjectRecord{}, false, nil
		}
		return LocalTicketObjectRecord{}, false, err
	}

	assigneeRows, err := s.db.QueryContext(ctx,
		`SELECT i.actor_id
		   FROM ff_ticket_assignee a
		   JOIN user_actor_identity i ON i.user_id = a.user_id
		  WHERE a.ticket_internal_id = $1::uuid
		  ORDER BY i.actor_id`,
		ticketInternalID,
	)
	if err != nil {
		return LocalTicketObjectRecord{}, false, err
	}
	defer assigneeRows.Close()

	assignedTo := []string{}
	for assigneeRows.Next() {
		var actorID string
		if err := assigneeRows.Scan(&actorID); err != nil {
			return LocalTicketObjectRecord{}, false, err
		}
		assignedTo = append(assignedTo, actorID)
	}
	if err := assigneeRows.Err(); err != nil {
		return LocalTicketObjectRecord{}, false, err
	}

	body, err := parseBody(record.BodyJSON)
	if err != nil {
		return LocalTicketObjectRecord{}, false, err
	}
	body["priority"] = priority
	if len(assignedTo) == 0 {
		delete(body, "assignedTo")
	} else {
		body["assignedTo"] = assignedTo
	}
	record.BodyJSON, err = json.Marshal(body)
	if err != nil {
		return LocalTicketObjectRecord{}, false, err
	}

	return record, true, nil
}

func (s *Store) GetLocalTicketCommentObjectBySlug(ctx context.Context, trackerSlug, ticketSlug, commentSlug string) (LocalTicketCommentObjectRecord, bool, error) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	ticketSlug = strings.TrimSpace(ticketSlug)
	commentSlug = strings.TrimSpace(commentSlug)
	if trackerSlug == "" || ticketSlug == "" || commentSlug == "" {
		return LocalTicketCommentObjectRecord{}, false, nil
	}

	var record LocalTicketCommentObjectRecord
	err := s.db.QueryRowContext(ctx,
		`SELECT c.note_primary_key,
		        n.body,
		        tr.slug,
		        t.slug,
		        c.slug,
		        r.slug
		   FROM ff_ticket_comment c
		   JOIN as_note n ON n.primary_key = c.note_primary_key
		   JOIN ff_ticket t ON t.internal_id = c.ticket_internal_id
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		   JOIN ff_repository r ON r.internal_id = c.repository_internal_id
		  WHERE tr.slug = $1
		    AND t.slug = $2
		    AND c.slug = $3`,
		trackerSlug,
		ticketSlug,
		commentSlug,
	).Scan(
		&record.PrimaryKey,
		&record.BodyJSON,
		&record.TrackerSlug,
		&record.TicketSlug,
		&record.CommentSlug,
		&record.RepositorySlug,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LocalTicketCommentObjectRecord{}, false, nil
		}
		return LocalTicketCommentObjectRecord{}, false, err
	}
	return record, true, nil
}

func (s *Store) ListTicketCommentsForTicket(ctx context.Context, userID int64, trackerSlug, ticketSlug string) ([]TicketComment, error) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	ticketSlug = strings.TrimSpace(ticketSlug)
	if userID <= 0 || trackerSlug == "" || ticketSlug == "" {
		return []TicketComment{}, nil
	}

	ticket, found, err := s.GetLocalTicketObjectBySlug(ctx, trackerSlug, ticketSlug)
	if err != nil {
		return nil, err
	}
	if !found {
		return []TicketComment{}, nil
	}
	canAccess, err := s.CanAccessRepository(ctx, ticket.RepositorySlug, userID)
	if err != nil {
		return nil, err
	}
	if !canAccess {
		return nil, fmt.Errorf("%w: repository access denied", ErrValidation)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id,
		        c.internal_id::text,
		        c.slug,
		        c.note_primary_key,
		        COALESCE(c.in_reply_to_note_primary_key, ''),
		        c.recipient_actor_id,
		        n.body,
		        tr.slug,
		        t.slug,
		        r.slug,
		        t.primary_key
		   FROM ff_ticket_comment c
		   JOIN as_note n ON n.primary_key = c.note_primary_key
		   JOIN ff_ticket t ON t.internal_id = c.ticket_internal_id
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		   JOIN ff_repository r ON r.internal_id = c.repository_internal_id
		  WHERE tr.slug = $1
		    AND t.slug = $2
		  ORDER BY c.created_at ASC, c.id ASC`,
		trackerSlug,
		ticketSlug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := []TicketComment{}
	for rows.Next() {
		var (
			c                TicketComment
			inReplyToNotePK  string
			bodyRaw          []byte
			ticketPrimaryKey string
		)
		if err := rows.Scan(
			&c.ID,
			&c.InternalID,
			&c.Slug,
			&c.ActorID,
			&inReplyToNotePK,
			&c.RecipientActorID,
			&bodyRaw,
			&c.TrackerSlug,
			&c.TicketSlug,
			&c.RepositorySlug,
			&ticketPrimaryKey,
		); err != nil {
			return nil, err
		}
		body, err := parseBody(bodyRaw)
		if err != nil {
			return nil, err
		}
		c.AttributedTo = stringFromAny(body["attributedTo"])
		c.Content = stringFromAny(body["content"])
		c.Published = stringFromAny(body["published"])
		if strings.TrimSpace(inReplyToNotePK) == "" {
			c.InReplyToActorID = ticketPrimaryKey
			c.InReplyToTicketID = true
		} else {
			c.InReplyToActorID = strings.TrimSpace(inReplyToNotePK)
			c.InReplyToTicketID = c.InReplyToActorID == ticketPrimaryKey
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return comments, nil
}

func (s *Store) CanAccessTicket(ctx context.Context, userID int64, trackerSlug, ticketSlug string) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	record, found, err := s.GetLocalTicketObjectBySlug(ctx, trackerSlug, ticketSlug)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return s.CanAccessRepository(ctx, record.RepositorySlug, userID)
}

func (s *Store) GetOAuthUserProfile(ctx context.Context, userID int64) (OAuthUserProfile, bool, error) {
	if userID <= 0 {
		return OAuthUserProfile{}, false, nil
	}
	var profile OAuthUserProfile
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, i.actor_id, i.main_key_id
		   FROM users u
		   JOIN user_actor_identity i ON i.user_id = u.id
		  WHERE u.id = $1`,
		userID,
	).Scan(&profile.UserID, &profile.Username, &profile.ActorID, &profile.MainKeyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OAuthUserProfile{}, false, nil
		}
		return OAuthUserProfile{}, false, err
	}
	return profile, true, nil
}

var (
	_ fosite.ClientManager          = (*Store)(nil)
	_ oauth2.CoreStorage            = (*Store)(nil)
	_ oauth2.TokenRevocationStorage = (*Store)(nil)
	_ pkce.PKCERequestStorage       = (*Store)(nil)
)
