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

package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/ory/fosite"
)

type oauthPrincipal struct {
	UserID   int64
	Username string
	ClientID string
	Scopes   []string
}

type oauthPrincipalContextKey struct{}

var oauthPrincipalKey oauthPrincipalContextKey

func (app *application) requireOAuthBearer(next http.HandlerFunc, requiredScopes ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header == "" || !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			writeBearerUnauthorized(w)
			return
		}
		token := strings.TrimSpace(header[len("Bearer "):])
		if token == "" {
			writeBearerUnauthorized(w)
			return
		}

		tokenUse, accessRequest, err := app.oauth.Provider.IntrospectToken(
			r.Context(),
			token,
			fosite.AccessToken,
			&fosite.DefaultSession{},
			requiredScopes...,
		)
		if err != nil || tokenUse != fosite.AccessToken || accessRequest == nil || accessRequest.GetSession() == nil {
			writeBearerUnauthorized(w)
			return
		}

		subject := strings.TrimSpace(accessRequest.GetSession().GetSubject())
		if subject == "" {
			writeBearerUnauthorized(w)
			return
		}
		userID, err := strconv.ParseInt(subject, 10, 64)
		if err != nil || userID <= 0 {
			writeBearerUnauthorized(w)
			return
		}

		principal := oauthPrincipal{
			UserID:   userID,
			Username: strings.TrimSpace(accessRequest.GetSession().GetUsername()),
			ClientID: accessRequest.GetClient().GetID(),
			Scopes:   append([]string(nil), accessRequest.GetGrantedScopes()...),
		}
		ctx := context.WithValue(r.Context(), oauthPrincipalKey, principal)
		next(w, r.WithContext(ctx))
	}
}

func oauthPrincipalFromContext(ctx context.Context) (oauthPrincipal, bool) {
	principal, ok := ctx.Value(oauthPrincipalKey).(oauthPrincipal)
	if !ok {
		return oauthPrincipal{}, false
	}
	if principal.UserID <= 0 {
		return oauthPrincipal{}, false
	}
	return principal, true
}

func writeBearerUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="api", error="invalid_token"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
