// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL
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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/refinement-systems/BorealValley/src/internal/assets"
	"golang.org/x/crypto/argon2"
)

const (
	PostgresDSNEnv             = "BV_PG_DSN"
	AgentCommentKindField      = "borealValleyAgentCommentKind"
	AgentCommentKindAck        = "ack"
	AgentCommentKindCompletion = "completion"
)

type argon2Params struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
	saltLen int
}

var defaultParams = argon2Params{
	time:    1,
	memory:  64 * 1024,
	threads: 4,
	keyLen:  32,
	saltLen: 16,
}

// Store is the composition root. Each embedded sub-store owns its domain's
// queries; Store itself holds only cross-domain orchestration methods.
type Store struct {
	UserStore
	RepositoryStore
	TicketStore
	NotificationStore
	OAuthStore
	cfg RootConfig
}

type Repository struct {
	ID                      int64
	InternalID              string
	Slug                    string
	Path                    string
	ActorID                 string
	TicketTrackerInternalID string
	TicketTrackerSlug       string
	TicketTrackerActorID    string
}

type RepositoryMember struct {
	RepositorySlug string
	UserID         int64
	Username       string
	CreatedAt      time.Time
}

type TicketTracker struct {
	ID         int64
	InternalID string
	Slug       string
	Name       string
	Summary    string
	ActorID    string
}

type Ticket struct {
	ID             int64
	InternalID     string
	Slug           string
	ActorID        string
	TrackerSlug    string
	RepositorySlug string
	Summary        string
	Content        string
	Published      string
	CreatedAt      time.Time
	Priority       int
}

type AssignedTicket struct {
	ID             int64
	ActorID        string
	TrackerSlug    string
	TicketSlug     string
	RepositorySlug string
	Summary        string
	Content        string
	CreatedAt      time.Time
	Priority       int
	RespondedByMe  bool
}

type ObjectVersionRecord struct {
	ID                     int64
	ObjectPrimaryKey       string
	ObjectType             string
	BodyJSON               []byte
	SourceUpdatePrimaryKey string
	CreatedByUserID        int64
	CreatedAt              time.Time
}

type UpdateRecord struct {
	PrimaryKey       string
	ObjectPrimaryKey string
	ObjectType       string
	Content          string
	Published        time.Time
}

type TicketComment struct {
	ID                int64
	InternalID        string
	Slug              string
	ActorID           string
	TicketSlug        string
	TrackerSlug       string
	RepositorySlug    string
	InReplyToActorID  string
	InReplyToTicketID bool
	AttributedTo      string
	Content           string
	Published         string
	RecipientActorID  string
}

type TicketAssignee struct {
	UserID     int64
	Username   string
	ActorID    string
	AssignedAt time.Time
}

type NotificationAccount struct {
	ID       int64
	Username string
	ActorID  string
}

type Notification struct {
	ID             int64
	Type           string
	Unread         bool
	CreatedAt      time.Time
	TicketActorID  string
	TicketSlug     string
	TrackerSlug    string
	RepositorySlug string
	Account        NotificationAccount
}

type LocalObjectRecord struct {
	PrimaryKey string
	BodyJSON   []byte
}

type LocalTicketObjectRecord struct {
	PrimaryKey     string
	BodyJSON       []byte
	TrackerSlug    string
	TicketSlug     string
	RepositorySlug string
}

type LocalTicketCommentObjectRecord struct {
	PrimaryKey     string
	BodyJSON       []byte
	TrackerSlug    string
	TicketSlug     string
	CommentSlug    string
	RepositorySlug string
}

type ObjectTypeCount struct {
	Label string
	Count int64
}

type NotificationListOptions struct {
	MinID int64
	MaxID int64
	Limit int
}

type AssignedTicketListOptions struct {
	Limit                      int
	UnrespondedOnly            bool
	AgentCompletionPendingOnly bool
}

func ResolvePostgresDSN(flagValue string) (string, error) {
	dsn := strings.TrimSpace(flagValue)
	if dsn != "" {
		return dsn, nil
	}
	dsn = strings.TrimSpace(os.Getenv(PostgresDSNEnv))
	if dsn == "" {
		return "", fmt.Errorf("postgres dsn required: provide --pg-dsn or set %s", PostgresDSNEnv)
	}
	return dsn, nil
}

func StoreInit(pgDSN string, rootDir string) (*Store, error) {
	pgDSN = strings.TrimSpace(pgDSN)
	if pgDSN == "" {
		return nil, errors.New("postgres dsn required")
	}

	cfg, err := LoadRootConfig(rootDir)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", pgDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	if _, err := db.Exec(assets.SqlControlCreate); err != nil {
		db.Close()
		return nil, err
	}

	baseURL := CanonicalBaseURL(cfg)
	store := &Store{
		UserStore:         UserStore{db: db, baseURL: baseURL},
		RepositoryStore:   RepositoryStore{db: db, baseURL: baseURL, rootDir: rootDir},
		TicketStore:       TicketStore{db: db, baseURL: baseURL},
		NotificationStore: NotificationStore{db: db},
		OAuthStore:        OAuthStore{db: db},
		cfg:               cfg,
	}
	if err := store.ensureUserActorIdentityBackfill(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Config() RootConfig {
	return s.cfg
}

func (s *Store) Close() {
	if s == nil || s.UserStore.db == nil {
		return
	}
	_ = s.UserStore.db.Close()
}

func (s *Store) BaseURL() string {
	return CanonicalBaseURL(s.cfg)
}

func (s *Store) ListTicketVersions(ctx context.Context, userID int64, trackerSlug, ticketSlug string, limit int) ([]ObjectVersionRecord, error) {
	record, found, err := s.GetLocalTicketObjectBySlug(ctx, trackerSlug, ticketSlug)
	if err != nil {
		return nil, err
	}
	if !found {
		return []ObjectVersionRecord{}, nil
	}
	canAccess, err := s.CanAccessRepository(ctx, record.RepositorySlug, userID)
	if err != nil {
		return nil, err
	}
	if !canAccess {
		return nil, fmt.Errorf("%w: repository access denied", ErrValidation)
	}
	return s.listObjectVersionsByPrimaryKey(ctx, record.PrimaryKey, limit)
}

func (s *Store) ListTicketCommentVersions(ctx context.Context, userID int64, trackerSlug, ticketSlug, commentSlug string, limit int) ([]ObjectVersionRecord, error) {
	record, found, err := s.GetLocalTicketCommentObjectBySlug(ctx, trackerSlug, ticketSlug, commentSlug)
	if err != nil {
		return nil, err
	}
	if !found {
		return []ObjectVersionRecord{}, nil
	}
	canAccess, err := s.CanAccessTicket(ctx, userID, trackerSlug, ticketSlug)
	if err != nil {
		return nil, err
	}
	if !canAccess {
		return nil, fmt.Errorf("%w: repository access denied", ErrValidation)
	}
	return s.listObjectVersionsByPrimaryKey(ctx, record.PrimaryKey, limit)
}

func (s *Store) ListObjectTypeCounts(ctx context.Context) ([]ObjectTypeCount, error) {
	counts := make([]ObjectTypeCount, 0, len(objectTables))
	for _, table := range objectTables {
		// objectTables is a hardcoded allowlist; allowedTable enforces that invariant.
		// The identifier is also quoted defensively against any future changes.
		if !allowedTable(table) {
			return nil, fmt.Errorf("disallowed table name: %q", table)
		}
		var count int64
		query := fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)
		if err := s.UserStore.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		counts = append(counts, ObjectTypeCount{Label: tableToTypeName[table], Count: count})
	}
	sort.Slice(counts, func(i, j int) bool {
		return counts[i].Label < counts[j].Label
	})
	return counts, nil
}

func fakeHashWork(password string) {
	salt := make([]byte, defaultParams.saltLen)
	_, _ = rand.Read(salt)
	_ = argon2.IDKey([]byte(password), salt, defaultParams.time, defaultParams.memory, defaultParams.threads, defaultParams.keyLen)
}

func buildLocalRepositoryObject(actorID, name string) map[string]any {
	return map[string]any{
		"@context":   []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		"id":         actorID,
		"type":       "Repository",
		"name":       name,
		"inbox":      actorID + "/inbox",
		"outbox":     actorID + "/outbox",
		"mainBranch": actorID + "/branches/main",
	}
}

func buildLocalTicketTrackerObject(actorID, name, summary string) map[string]any {
	obj := map[string]any{
		"@context": []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		"id":       actorID,
		"type":     "TicketTracker",
		"name":     name,
		"inbox":    actorID + "/inbox",
		"outbox":   actorID + "/outbox",
	}
	if summary != "" {
		obj["summary"] = summary
	}
	return obj
}

func buildLocalTicketObject(actorID, trackerActorID, repoActorID, authorActorID, summary, content string, published time.Time) map[string]any {
	return map[string]any{
		"@context":     []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		"id":           actorID,
		"type":         "Ticket",
		"context":      trackerActorID,
		"target":       repoActorID,
		"attributedTo": authorActorID,
		"summary":      summary,
		"content":      content,
		"mediaType":    "text/plain",
		"source": map[string]any{
			"mediaType": "text/plain",
			"content":   content,
		},
		"published":    published.Format(time.RFC3339Nano),
		"isResolved":   false,
		"followers":    actorID + "/followers",
		"replies":      actorID + "/replies",
		"team":         actorID + "/team",
		"dependencies": actorID + "/dependencies",
		"dependants":   actorID + "/dependants",
	}
}

func buildLocalTicketCommentObject(actorID, ticketActorID, inReplyToActorID, authorActorID, recipientActorID, content, agentCommentKind string, published time.Time) map[string]any {
	obj := map[string]any{
		"@context":     []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		"id":           actorID,
		"type":         "Note",
		"context":      ticketActorID,
		"inReplyTo":    inReplyToActorID,
		"attributedTo": authorActorID,
		"to":           []string{recipientActorID},
		"content":      content,
		"mediaType":    "text/plain",
		"source": map[string]any{
			"mediaType": "text/plain",
			"content":   content,
		},
		"published": published.Format(time.RFC3339Nano),
	}
	if agentCommentKind != "" {
		obj[AgentCommentKindField] = agentCommentKind
	}
	return obj
}

func buildLocalUpdateObject(actorID, authorActorID, objectActorID, content string, published time.Time) map[string]any {
	return map[string]any{
		"@context":  []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		"id":        actorID,
		"type":      "Update",
		"actor":     authorActorID,
		"object":    objectActorID,
		"content":   content,
		"mediaType": "text/plain",
		"published": published.Format(time.RFC3339Nano),
	}
}

func upsertLocalRepositoryTx(ctx context.Context, tx *sql.Tx, actorID string, body map[string]any, internalID uuid.UUID, slug, fsPath string) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE ff_repository
		    SET primary_key = $1,
		        body = $2::jsonb,
		        updated_at = now(),
		        slug = $4,
		        filesystem_path = $5,
		        is_local = TRUE
		  WHERE internal_id = $3::uuid`,
		actorID, string(raw), internalID.String(), slug, fsPath,
	)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		return nil
	}

	result, err = tx.ExecContext(ctx,
		`UPDATE ff_repository
		    SET primary_key = $1,
		        body = $2::jsonb,
		        updated_at = now(),
		        slug = $3,
		        filesystem_path = $4,
		        is_local = TRUE
		  WHERE slug = $3
		     OR filesystem_path = $4`,
		actorID, string(raw), slug, fsPath,
	)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		return nil
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO ff_repository (primary_key, body, created_at, updated_at, internal_id, slug, filesystem_path, is_local)
	         VALUES ($1, $2::jsonb, now(), now(), $3::uuid, $4, $5, TRUE)`,
		actorID, string(raw), internalID.String(), slug, fsPath,
	)
	return err
}

type repositoryRow struct {
	InternalID              string
	ActorID                 string
	Body                    []byte
	Slug                    string
	TicketTrackerInternalID string
}

type trackerRow struct {
	InternalID string
	ActorID    string
	Body       []byte
	Slug       string
}

type ticketRow struct {
	InternalID        string
	ActorID           string
	Body              []byte
	Slug              string
	TrackerInternalID string
	TrackerSlug       string
	RepositoryID      string
	RepositorySlug    string
}

type ticketCommentRow struct {
	InternalID           string
	Slug                 string
	NotePrimaryKey       string
	TicketInternalID     string
	RepositoryInternalID string
	InReplyToNotePK      string
	RecipientActorID     string
	Body                 []byte
}

type assigneeLookupRow struct {
	UserID  int64
	ActorID string
}

func loadAssignableUserByUsernameTx(ctx context.Context, tx *sql.Tx, username, repositoryInternalID string) (assigneeLookupRow, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return assigneeLookupRow{}, fmt.Errorf("%w: username is required", ErrValidation)
	}

	var row assigneeLookupRow
	err := tx.QueryRowContext(ctx,
		`SELECT u.id, i.actor_id
		   FROM users u
		   JOIN user_actor_identity i ON i.user_id = u.id
		  WHERE u.username = $1`,
		username,
	).Scan(&row.UserID, &row.ActorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return assigneeLookupRow{}, fmt.Errorf("%w: user not found", ErrValidation)
		}
		return assigneeLookupRow{}, err
	}

	canAccess, err := canAccessRepositoryTx(ctx, tx, repositoryInternalID, row.UserID)
	if err != nil {
		return assigneeLookupRow{}, err
	}
	if !canAccess {
		return assigneeLookupRow{}, fmt.Errorf("%w: assignee has no repository access", ErrValidation)
	}
	return row, nil
}

func loadRepositoryRowBySlugTx(ctx context.Context, tx *sql.Tx, slug string) (repositoryRow, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return repositoryRow{}, fmt.Errorf("%w: repository slug is required", ErrValidation)
	}

	var row repositoryRow
	err := tx.QueryRowContext(ctx,
		`SELECT internal_id::text, primary_key, body, slug, COALESCE(ticket_tracker_internal_id::text, '')
		   FROM ff_repository
		  WHERE is_local = TRUE AND slug = $1
		  FOR UPDATE`,
		slug,
	).Scan(&row.InternalID, &row.ActorID, &row.Body, &row.Slug, &row.TicketTrackerInternalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repositoryRow{}, fmt.Errorf("%w: repository not found", ErrValidation)
		}
		return repositoryRow{}, err
	}
	return row, nil
}

func loadTrackerRowBySlugTx(ctx context.Context, tx *sql.Tx, slug string) (trackerRow, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return trackerRow{}, fmt.Errorf("%w: ticket tracker slug is required", ErrValidation)
	}

	var row trackerRow
	err := tx.QueryRowContext(ctx,
		`SELECT internal_id::text, primary_key, body, slug
		   FROM ff_ticket_tracker
		  WHERE is_local = TRUE AND slug = $1
		  FOR UPDATE`,
		slug,
	).Scan(&row.InternalID, &row.ActorID, &row.Body, &row.Slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return trackerRow{}, fmt.Errorf("%w: ticket tracker not found", ErrValidation)
		}
		return trackerRow{}, err
	}
	return row, nil
}

func loadTrackerRowByInternalIDTx(ctx context.Context, tx *sql.Tx, internalID string) (trackerRow, error) {
	var row trackerRow
	err := tx.QueryRowContext(ctx,
		`SELECT internal_id::text, primary_key, body, slug
		   FROM ff_ticket_tracker
		  WHERE is_local = TRUE AND internal_id = $1::uuid
		  FOR UPDATE`,
		internalID,
	).Scan(&row.InternalID, &row.ActorID, &row.Body, &row.Slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return trackerRow{}, fmt.Errorf("%w: ticket tracker not found", ErrValidation)
		}
		return trackerRow{}, err
	}
	return row, nil
}

func loadTicketRowBySlugsTx(ctx context.Context, tx *sql.Tx, trackerSlug, ticketSlug string) (ticketRow, error) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	ticketSlug = strings.TrimSpace(ticketSlug)
	if trackerSlug == "" {
		return ticketRow{}, fmt.Errorf("%w: ticket tracker slug is required", ErrValidation)
	}
	if ticketSlug == "" {
		return ticketRow{}, fmt.Errorf("%w: ticket slug is required", ErrValidation)
	}
	var row ticketRow
	err := tx.QueryRowContext(ctx,
		`SELECT t.internal_id::text,
		        t.primary_key,
		        t.body,
		        t.slug,
		        tr.internal_id::text,
		        tr.slug,
		        r.internal_id::text,
		        r.slug
		   FROM ff_ticket t
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		   JOIN ff_repository r ON r.internal_id = t.repository_internal_id
		  WHERE t.is_local = TRUE
		    AND tr.slug = $1
		    AND t.slug = $2
		  FOR UPDATE`,
		trackerSlug, ticketSlug,
	).Scan(
		&row.InternalID,
		&row.ActorID,
		&row.Body,
		&row.Slug,
		&row.TrackerInternalID,
		&row.TrackerSlug,
		&row.RepositoryID,
		&row.RepositorySlug,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ticketRow{}, fmt.Errorf("%w: ticket not found", ErrValidation)
		}
		return ticketRow{}, err
	}
	return row, nil
}

func loadTicketCommentRowByActorIDTx(ctx context.Context, tx *sql.Tx, actorID string) (ticketCommentRow, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ticketCommentRow{}, fmt.Errorf("%w: in_reply_to is required", ErrValidation)
	}
	var row ticketCommentRow
	err := tx.QueryRowContext(ctx,
		`SELECT c.internal_id::text,
		        c.slug,
		        c.note_primary_key,
		        c.ticket_internal_id::text,
		        c.repository_internal_id::text,
		        COALESCE(c.in_reply_to_note_primary_key, ''),
		        c.recipient_actor_id,
		        n.body
		   FROM ff_ticket_comment c
		   JOIN as_note n ON n.primary_key = c.note_primary_key
		  WHERE c.note_primary_key = $1
		  FOR UPDATE`,
		actorID,
	).Scan(
		&row.InternalID,
		&row.Slug,
		&row.NotePrimaryKey,
		&row.TicketInternalID,
		&row.RepositoryInternalID,
		&row.InReplyToNotePK,
		&row.RecipientActorID,
		&row.Body,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ticketCommentRow{}, fmt.Errorf("%w: parent comment not found", ErrValidation)
		}
		return ticketCommentRow{}, err
	}
	return row, nil
}

func loadTicketCommentRowBySlugsTx(ctx context.Context, tx *sql.Tx, trackerSlug, ticketSlug, commentSlug string) (ticketCommentRow, error) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	ticketSlug = strings.TrimSpace(ticketSlug)
	commentSlug = strings.TrimSpace(commentSlug)
	if trackerSlug == "" {
		return ticketCommentRow{}, fmt.Errorf("%w: ticket tracker slug is required", ErrValidation)
	}
	if ticketSlug == "" {
		return ticketCommentRow{}, fmt.Errorf("%w: ticket slug is required", ErrValidation)
	}
	if commentSlug == "" {
		return ticketCommentRow{}, fmt.Errorf("%w: comment slug is required", ErrValidation)
	}

	var row ticketCommentRow
	err := tx.QueryRowContext(ctx,
		`SELECT c.internal_id::text,
		        c.slug,
		        c.note_primary_key,
		        c.ticket_internal_id::text,
		        c.repository_internal_id::text,
		        COALESCE(c.in_reply_to_note_primary_key, ''),
		        c.recipient_actor_id,
		        n.body
		   FROM ff_ticket_comment c
		   JOIN as_note n ON n.primary_key = c.note_primary_key
		   JOIN ff_ticket t ON t.internal_id = c.ticket_internal_id
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		  WHERE tr.slug = $1
		    AND t.slug = $2
		    AND c.slug = $3
		  FOR UPDATE`,
		trackerSlug,
		ticketSlug,
		commentSlug,
	).Scan(
		&row.InternalID,
		&row.Slug,
		&row.NotePrimaryKey,
		&row.TicketInternalID,
		&row.RepositoryInternalID,
		&row.InReplyToNotePK,
		&row.RecipientActorID,
		&row.Body,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ticketCommentRow{}, fmt.Errorf("%w: ticket comment not found", ErrValidation)
		}
		return ticketCommentRow{}, err
	}
	return row, nil
}

func loadUserActorIDTx(ctx context.Context, tx *sql.Tx, userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("%w: user id is required", ErrValidation)
	}
	var actorID string
	if err := tx.QueryRowContext(ctx,
		`SELECT actor_id FROM user_actor_identity WHERE user_id = $1`,
		userID,
	).Scan(&actorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: actor identity not found", ErrValidation)
		}
		return "", err
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "", fmt.Errorf("%w: actor identity not found", ErrValidation)
	}
	return actorID, nil
}

func snapshotObjectVersionTx(ctx context.Context, tx *sql.Tx, objectPrimaryKey, objectType string, bodyRaw []byte, sourceUpdatePrimaryKey string, createdByUserID int64) error {
	if strings.TrimSpace(objectPrimaryKey) == "" {
		return fmt.Errorf("%w: object primary key is required", ErrValidation)
	}
	if strings.TrimSpace(objectType) == "" {
		return fmt.Errorf("%w: object type is required", ErrValidation)
	}
	if len(bodyRaw) == 0 {
		return fmt.Errorf("%w: object body is required", ErrValidation)
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO ap_object_version (
			object_primary_key,
			object_type,
			body,
			source_update_primary_key,
			created_by_user_id,
			created_at
		) VALUES (
			$1,
			$2,
			$3::jsonb,
			NULLIF($4, ''),
			$5,
			now()
		)`,
		objectPrimaryKey,
		objectType,
		string(bodyRaw),
		strings.TrimSpace(sourceUpdatePrimaryKey),
		createdByUserID,
	)
	return err
}

func ensureRepositoryMemberTx(ctx context.Context, tx *sql.Tx, repositoryInternalID string, userID int64) error {
	if repositoryInternalID == "" || userID <= 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO ff_repository_member (repository_internal_id, user_id, created_at)
	         VALUES ($1::uuid, $2, now())
	      ON CONFLICT (repository_internal_id, user_id) DO NOTHING`,
		repositoryInternalID,
		userID,
	)
	return err
}

func appendPlainTextUpdate(bodyRaw []byte, updateContent string, published time.Time) ([]byte, error) {
	body, err := parseBody(bodyRaw)
	if err != nil {
		return nil, err
	}

	updateContent = strings.TrimSpace(updateContent)
	if updateContent == "" {
		return nil, fmt.Errorf("%w: content is required", ErrValidation)
	}

	existing := strings.TrimSpace(stringFromAny(body["content"]))
	merged := updateContent
	if existing != "" {
		merged = existing + "\n\n" + updateContent
	}
	body["content"] = merged
	body["mediaType"] = "text/plain"
	body["updated"] = published.Format(time.RFC3339Nano)

	if src, ok := body["source"].(map[string]any); ok {
		src["content"] = merged
		if strings.TrimSpace(stringFromAny(src["mediaType"])) == "" {
			src["mediaType"] = "text/plain"
		}
		body["source"] = src
	} else {
		body["source"] = map[string]any{
			"mediaType": "text/plain",
			"content":   merged,
		}
	}

	return json.Marshal(body)
}

func parseBody(raw []byte) (map[string]any, error) {
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func stringSliceFromAny(raw any) []string {
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(item)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func stringFromAny(raw any) string {
	s, _ := raw.(string)
	return strings.TrimSpace(s)
}

func addUniqueString(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func removeString(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	filtered := make([]string, 0, len(items))
	for _, existing := range items {
		if existing != item {
			filtered = append(filtered, existing)
		}
	}
	return filtered
}

func containsString(items []string, item string) bool {
	for _, existing := range items {
		if existing == item {
			return true
		}
	}
	return false
}

func reverseNotifications(items []Notification) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func nullString(value, treatAsNull string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.TrimSpace(treatAsNull) != "" && value == strings.TrimSpace(treatAsNull) {
		return ""
	}
	return value
}

func canAccessRepositoryTx(ctx context.Context, tx *sql.Tx, repositoryInternalID string, userID int64) (bool, error) {
	if repositoryInternalID == "" || userID <= 0 {
		return false, nil
	}
	var isAdmin bool
	err := tx.QueryRowContext(ctx,
		`SELECT is_admin FROM users WHERE id = $1`,
		userID,
	).Scan(&isAdmin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if isAdmin {
		return true, nil
	}
	var isMember bool
	err = tx.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1
			  FROM ff_repository_member
			 WHERE repository_internal_id = $1::uuid
			   AND user_id = $2
		)`,
		repositoryInternalID, userID,
	).Scan(&isMember)
	if err != nil {
		return false, err
	}
	return isMember, nil
}

func ensureStableUUID(dir string, fileName string) (uuid.UUID, error) {
	marker := filepath.Join(dir, fileName)
	raw, err := os.ReadFile(marker)
	if err == nil {
		id, parseErr := uuid.Parse(strings.TrimSpace(string(raw)))
		if parseErr != nil {
			return uuid.Nil, fmt.Errorf("invalid UUID marker %q: %w", marker, parseErr)
		}
		return id, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return uuid.Nil, err
	}

	id := uuid.New()
	if writeErr := os.WriteFile(marker, []byte(id.String()+"\n"), 0o600); writeErr != nil {
		return uuid.Nil, writeErr
	}
	return id, nil
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = slugNonAlnum.ReplaceAllString(raw, "-")
	raw = strings.Trim(raw, "-")
	if raw == "" {
		return "unnamed"
	}
	return raw
}

func withTx(db *sql.DB, ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
