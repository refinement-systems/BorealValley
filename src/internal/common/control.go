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
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

type Store struct {
	db      *sql.DB
	rootDir string
	cfg     RootConfig
}

func (s *Store) Config() RootConfig {
	return s.cfg
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
	Table string
	Count int64
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

	store := &Store{db: db, rootDir: rootDir, cfg: cfg}
	if err := store.ensureUserActorIdentityBackfill(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() {
	if s == nil || s.db == nil {
		return
	}
	_ = s.db.Close()
}

func (s *Store) BaseURL() string {
	return CanonicalBaseURL(s.cfg)
}

func (s *Store) CreateUser(ctx context.Context, username, password string) error {
	return s.CreateUserWithAdmin(ctx, username, password, false)
}

func (s *Store) CreateUserWithAdmin(ctx context.Context, username, password string, isAdmin bool) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("empty username")
	}
	if len(password) < 12 {
		return errors.New("password too short (min 12 chars)")
	}

	salt := make([]byte, defaultParams.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}

	hash := argon2.IDKey([]byte(password), salt, defaultParams.time, defaultParams.memory, defaultParams.threads, defaultParams.keyLen)
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var userID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (username, password_hash, salt, is_admin, created_at)
	         VALUES ($1, $2, $3, $4, now())
	      RETURNING id`,
		username, hashB64, saltB64, isAdmin,
	).Scan(&userID)
	if err != nil {
		return err
	}

	if err := provisionUserActorTx(ctx, tx, userID, username, s.BaseURL()); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) IsUserAdmin(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	var isAdmin bool
	err := s.db.QueryRowContext(ctx,
		`SELECT is_admin FROM users WHERE id = $1`,
		userID,
	).Scan(&isAdmin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return isAdmin, nil
}

func (s *Store) VerifyUser(ctx context.Context, username, password string) (int64, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return 0, false, nil
	}

	var (
		id      int64
		hashB64 string
		saltB64 string
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT id, password_hash, salt FROM users WHERE username = $1`,
		username,
	).Scan(&id, &hashB64, &saltB64)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fakeHashWork(password)
			return 0, false, nil
		}
		return 0, false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(saltB64)
	if err != nil {
		return 0, false, err
	}
	wantHash, err := base64.RawStdEncoding.DecodeString(hashB64)
	if err != nil {
		return 0, false, err
	}

	gotHash := argon2.IDKey([]byte(password), salt, defaultParams.time, defaultParams.memory, defaultParams.threads, defaultParams.keyLen)
	if len(gotHash) != len(wantHash) {
		return 0, false, nil
	}
	if subtle.ConstantTimeCompare(gotHash, wantHash) != 1 {
		return 0, false, nil
	}
	return id, true, nil
}

func fakeHashWork(password string) {
	salt := make([]byte, defaultParams.saltLen)
	_, _ = rand.Read(salt)
	_ = argon2.IDKey([]byte(password), salt, defaultParams.time, defaultParams.memory, defaultParams.threads, defaultParams.keyLen)
}

func (s *Store) ResyncFromFilesystem(ctx context.Context) error {
	repoPath := RootRepoPath(s.rootDir)
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE ff_repository SET is_local = FALSE, updated_at = now() WHERE is_local = TRUE`); err != nil {
		return err
	}

	slugCollision := map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		repoDir := filepath.Join(repoPath, entry.Name())
		if !IsGitRepo(repoDir) {
			continue
		}

		repoSlug := slugify(entry.Name())
		if priorPath, ok := slugCollision[repoSlug]; ok {
			return fmt.Errorf("repository slug collision: %q and %q both map to %q", priorPath, repoDir, repoSlug)
		}
		slugCollision[repoSlug] = repoDir

		repoID, err := ensureStableUUID(repoDir, RepoIDFileName)
		if err != nil {
			return err
		}

		repoActorID := s.BaseURL() + "/repo/" + repoSlug
		repoBody := buildLocalRepositoryObject(repoActorID, entry.Name())
		if err := upsertLocalRepositoryTx(ctx, tx, repoActorID, repoBody, repoID, repoSlug, repoDir); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ListRepositories(ctx context.Context) ([]Repository, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id,
		        r.internal_id::text,
		        r.slug,
		        r.filesystem_path,
		        r.primary_key,
		        COALESCE(r.ticket_tracker_internal_id::text, ''),
		        COALESCE(t.slug, ''),
		        COALESCE(t.primary_key, '')
		   FROM ff_repository r
		   LEFT JOIN ff_ticket_tracker t ON t.internal_id = r.ticket_tracker_internal_id
		  WHERE r.is_local = TRUE
		  ORDER BY r.slug`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	repos := []Repository{}
	for rows.Next() {
		var r Repository
		if err := rows.Scan(
			&r.ID,
			&r.InternalID,
			&r.Slug,
			&r.Path,
			&r.ActorID,
			&r.TicketTrackerInternalID,
			&r.TicketTrackerSlug,
			&r.TicketTrackerActorID,
		); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return repos, nil
}

func (s *Store) GetRepositoryBySlug(ctx context.Context, repoSlug string) (Repository, bool, error) {
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" {
		return Repository{}, false, nil
	}

	var r Repository
	err := s.db.QueryRowContext(ctx,
		`SELECT r.id,
		        r.internal_id::text,
		        r.slug,
		        r.filesystem_path,
		        r.primary_key,
		        COALESCE(r.ticket_tracker_internal_id::text, ''),
		        COALESCE(t.slug, ''),
		        COALESCE(t.primary_key, '')
		   FROM ff_repository r
		   LEFT JOIN ff_ticket_tracker t ON t.internal_id = r.ticket_tracker_internal_id
		  WHERE r.is_local = TRUE AND r.slug = $1`,
		repoSlug,
	).Scan(
		&r.ID,
		&r.InternalID,
		&r.Slug,
		&r.Path,
		&r.ActorID,
		&r.TicketTrackerInternalID,
		&r.TicketTrackerSlug,
		&r.TicketTrackerActorID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Repository{}, false, nil
		}
		return Repository{}, false, err
	}
	return r, true, nil
}

func (s *Store) ListRepositoryMembers(ctx context.Context, repoSlug string) ([]RepositoryMember, error) {
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" {
		return []RepositoryMember{}, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.slug, u.id, u.username, m.created_at
		   FROM ff_repository_member m
		   JOIN ff_repository r ON r.internal_id = m.repository_internal_id
		   JOIN users u ON u.id = m.user_id
		  WHERE r.is_local = TRUE AND r.slug = $1
		  ORDER BY u.username`,
		repoSlug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []RepositoryMember{}
	for rows.Next() {
		var m RepositoryMember
		if err := rows.Scan(&m.RepositorySlug, &m.UserID, &m.Username, &m.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}

func (s *Store) AddRepositoryMemberByUsername(ctx context.Context, repoSlug, username string) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		repo, err := loadRepositoryRowBySlugTx(ctx, tx, repoSlug)
		if err != nil {
			return err
		}
		var userID int64
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM users WHERE username = $1`,
			strings.TrimSpace(username),
		).Scan(&userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: user not found", ErrValidation)
			}
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO ff_repository_member (repository_internal_id, user_id, created_at)
		         VALUES ($1::uuid, $2, now())
		      ON CONFLICT (repository_internal_id, user_id) DO NOTHING`,
			repo.InternalID, userID,
		); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) RemoveRepositoryMemberByUsername(ctx context.Context, repoSlug, username string) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		repo, err := loadRepositoryRowBySlugTx(ctx, tx, repoSlug)
		if err != nil {
			return err
		}
		var userID int64
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM users WHERE username = $1`,
			strings.TrimSpace(username),
		).Scan(&userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: user not found", ErrValidation)
			}
			return err
		}
		_, err = tx.ExecContext(ctx,
			`DELETE FROM ff_repository_member
			  WHERE repository_internal_id = $1::uuid
			    AND user_id = $2`,
			repo.InternalID, userID,
		)
		return err
	})
}

func (s *Store) IsRepositoryMember(ctx context.Context, repoSlug string, userID int64) (bool, error) {
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" || userID <= 0 {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
		       SELECT 1
		         FROM ff_repository_member m
		         JOIN ff_repository r ON r.internal_id = m.repository_internal_id
		        WHERE r.is_local = TRUE AND r.slug = $1 AND m.user_id = $2
		)`,
		repoSlug, userID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) CanAccessRepository(ctx context.Context, repoSlug string, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	isAdmin, err := s.IsUserAdmin(ctx, userID)
	if err != nil {
		return false, err
	}
	if isAdmin {
		return true, nil
	}
	return s.IsRepositoryMember(ctx, repoSlug, userID)
}

func (s *Store) ListTicketTrackers(ctx context.Context) ([]TicketTracker, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, internal_id::text, slug, body, primary_key
		   FROM ff_ticket_tracker
		  WHERE is_local = TRUE
		  ORDER BY slug`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trackers := []TicketTracker{}
	for rows.Next() {
		var (
			t       TicketTracker
			bodyRaw []byte
		)
		if err := rows.Scan(&t.ID, &t.InternalID, &t.Slug, &bodyRaw, &t.ActorID); err != nil {
			return nil, err
		}
		body, err := parseBody(bodyRaw)
		if err != nil {
			return nil, err
		}
		t.Name = stringFromAny(body["name"])
		t.Summary = stringFromAny(body["summary"])
		trackers = append(trackers, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return trackers, nil
}

func (s *Store) GetTicketTrackerBySlug(ctx context.Context, trackerSlug string) (TicketTracker, bool, error) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	if trackerSlug == "" {
		return TicketTracker{}, false, nil
	}

	var t TicketTracker
	var bodyRaw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, internal_id::text, slug, body, primary_key
		   FROM ff_ticket_tracker
		  WHERE is_local = TRUE AND slug = $1`,
		trackerSlug,
	).Scan(&t.ID, &t.InternalID, &t.Slug, &bodyRaw, &t.ActorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TicketTracker{}, false, nil
		}
		return TicketTracker{}, false, err
	}
	body, err := parseBody(bodyRaw)
	if err != nil {
		return TicketTracker{}, false, err
	}
	t.Name = stringFromAny(body["name"])
	t.Summary = stringFromAny(body["summary"])
	return t, true, nil
}

func (s *Store) ListRepositoriesForTracker(ctx context.Context, trackerSlug string) ([]Repository, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id,
		        r.internal_id::text,
		        r.slug,
		        r.filesystem_path,
		        r.primary_key,
		        COALESCE(r.ticket_tracker_internal_id::text, ''),
		        COALESCE(t.slug, ''),
		        COALESCE(t.primary_key, '')
		   FROM ff_repository r
		   JOIN ff_ticket_tracker t ON t.internal_id = r.ticket_tracker_internal_id
		  WHERE r.is_local = TRUE AND t.slug = $1
		  ORDER BY r.slug`,
		trackerSlug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	repos := []Repository{}
	for rows.Next() {
		var r Repository
		if err := rows.Scan(
			&r.ID,
			&r.InternalID,
			&r.Slug,
			&r.Path,
			&r.ActorID,
			&r.TicketTrackerInternalID,
			&r.TicketTrackerSlug,
			&r.TicketTrackerActorID,
		); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return repos, nil
}

func (s *Store) CreateTicketTracker(ctx context.Context, userID int64, name, summary string) (TicketTracker, error) {
	name = strings.TrimSpace(name)
	summary = strings.TrimSpace(summary)
	if name == "" {
		return TicketTracker{}, fmt.Errorf("%w: name is required", ErrValidation)
	}

	slug := slugify(name)
	internalID := uuid.New()
	actorID := s.BaseURL() + "/ticket-tracker/" + slug
	body := buildLocalTicketTrackerObject(actorID, name, summary)
	raw, err := json.Marshal(body)
	if err != nil {
		return TicketTracker{}, err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO ff_ticket_tracker (primary_key, body, created_at, updated_at, internal_id, slug, created_by_user_id, is_local)
	         VALUES ($1, $2::jsonb, now(), now(), $3::uuid, $4, $5, TRUE)`,
		actorID, string(raw), internalID.String(), slug, userID,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			return TicketTracker{}, fmt.Errorf("%w: ticket tracker slug already exists", ErrValidation)
		}
		return TicketTracker{}, err
	}

	tracker, found, err := s.GetTicketTrackerBySlug(ctx, slug)
	if err != nil {
		return TicketTracker{}, err
	}
	if !found {
		return TicketTracker{}, errors.New("created ticket tracker not found")
	}
	return tracker, nil
}

func (s *Store) AssignTicketTrackerToRepository(ctx context.Context, repoSlug, trackerSlug string) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		repo, err := loadRepositoryRowBySlugTx(ctx, tx, repoSlug)
		if err != nil {
			return err
		}
		tracker, err := loadTrackerRowBySlugTx(ctx, tx, trackerSlug)
		if err != nil {
			return err
		}
		if repo.TicketTrackerInternalID == tracker.InternalID {
			return nil
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE ff_repository
			    SET ticket_tracker_internal_id = $1::uuid,
			        updated_at = now()
			  WHERE internal_id = $2::uuid`,
			tracker.InternalID, repo.InternalID,
		); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) UnassignTicketTrackerFromRepository(ctx context.Context, repoSlug string) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		repo, err := loadRepositoryRowBySlugTx(ctx, tx, repoSlug)
		if err != nil {
			return err
		}
		if repo.TicketTrackerInternalID == "" {
			return nil
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE ff_repository
			    SET ticket_tracker_internal_id = NULL,
			        updated_at = now()
			  WHERE internal_id = $2::uuid`,
			repo.InternalID,
		); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) CreateTicket(ctx context.Context, userID int64, trackerSlug, repoSlug, summary, content string) (Ticket, error) {
	return s.CreateTicketWithPriority(ctx, userID, trackerSlug, repoSlug, summary, content, 0)
}

func (s *Store) CreateTicketWithPriority(ctx context.Context, userID int64, trackerSlug, repoSlug, summary, content string, priority int) (Ticket, error) {
	summary = strings.TrimSpace(summary)
	content = strings.TrimSpace(content)
	if summary == "" {
		return Ticket{}, fmt.Errorf("%w: summary is required", ErrValidation)
	}
	if content == "" {
		return Ticket{}, fmt.Errorf("%w: content is required", ErrValidation)
	}
	if priority < 0 {
		return Ticket{}, fmt.Errorf("%w: priority cannot be negative", ErrValidation)
	}
	if priority > 2147483647 {
		return Ticket{}, fmt.Errorf("%w: priority exceeds max supported value", ErrValidation)
	}

	var ticket Ticket
	err := withTx(s.db, ctx, func(tx *sql.Tx) error {
		tracker, err := loadTrackerRowBySlugTx(ctx, tx, trackerSlug)
		if err != nil {
			return err
		}
		repo, err := loadRepositoryRowBySlugTx(ctx, tx, repoSlug)
		if err != nil {
			return err
		}
		if repo.TicketTrackerInternalID != tracker.InternalID {
			return fmt.Errorf("%w: repository is not assigned to this ticket tracker", ErrValidation)
		}

		if err := ensureRepositoryMemberTx(ctx, tx, repo.InternalID, userID); err != nil {
			return err
		}

		var authorActorID string
		if err := tx.QueryRowContext(ctx,
			`SELECT actor_id FROM user_actor_identity WHERE user_id = $1`,
			userID,
		).Scan(&authorActorID); err != nil {
			return err
		}

		ticketSlug := NewTicketID()
		ticketActorID := s.BaseURL() + "/ticket-tracker/" + tracker.Slug + "/ticket/" + ticketSlug
		ticketInternalID := uuid.New()
		published := time.Now().UTC()
		ticketBody := buildLocalTicketObject(ticketActorID, tracker.ActorID, repo.ActorID, authorActorID, summary, content, published)
		raw, err := json.Marshal(ticketBody)
		if err != nil {
			return err
		}

		if err := tx.QueryRowContext(ctx,
			`INSERT INTO ff_ticket (primary_key, body, created_at, updated_at, internal_id, slug, tracker_internal_id, repository_internal_id, created_by_user_id, priority, is_local)
		         VALUES ($1, $2::jsonb, now(), now(), $3::uuid, $4, $5::uuid, $6::uuid, $7, $8, TRUE)
		      RETURNING id, created_at`,
			ticketActorID,
			string(raw),
			ticketInternalID.String(),
			ticketSlug,
			tracker.InternalID,
			repo.InternalID,
			userID,
			priority,
		).Scan(&ticket.ID, &ticket.CreatedAt); err != nil {
			return err
		}

		ticket.InternalID = ticketInternalID.String()
		ticket.Slug = ticketSlug
		ticket.ActorID = ticketActorID
		ticket.TrackerSlug = tracker.Slug
		ticket.RepositorySlug = repo.Slug
		ticket.Summary = summary
		ticket.Content = content
		ticket.Published = published.Format(time.RFC3339Nano)
		ticket.Priority = priority
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return ticket, nil
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

func (s *Store) UpdateTicketAssigneeByUsername(ctx context.Context, actorUserID int64, trackerSlug, ticketSlug, action, username string) error {
	action = strings.TrimSpace(action)
	username = strings.TrimSpace(username)
	if actorUserID <= 0 {
		return fmt.Errorf("%w: user id is required", ErrValidation)
	}
	if username == "" {
		return fmt.Errorf("%w: username is required", ErrValidation)
	}
	if action == "" {
		action = "add"
	}

	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		ticket, err := loadTicketRowBySlugsTx(ctx, tx, trackerSlug, ticketSlug)
		if err != nil {
			return err
		}
		canAccess, err := canAccessRepositoryTx(ctx, tx, ticket.RepositoryID, actorUserID)
		if err != nil {
			return err
		}
		if !canAccess {
			return fmt.Errorf("%w: repository access denied", ErrValidation)
		}

		assignee, err := loadAssignableUserByUsernameTx(ctx, tx, username, ticket.RepositoryID)
		if err != nil {
			return err
		}

		switch action {
		case "add":
			result, err := tx.ExecContext(ctx,
				`INSERT INTO ff_ticket_assignee (ticket_internal_id, user_id, assigned_by_user_id, created_at, updated_at)
			         VALUES ($1::uuid, $2, $3, now(), now())
			      ON CONFLICT (ticket_internal_id, user_id) DO NOTHING`,
				ticket.InternalID, assignee.UserID, actorUserID,
			)
			if err != nil {
				return err
			}
			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if rowsAffected == 0 {
				return nil
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO notification (user_id, kind, ticket_internal_id, repository_internal_id, assigned_by_user_id, unread, created_at, updated_at)
				         VALUES ($1, 'ticket_assigned', $2::uuid, $3::uuid, $4, TRUE, now(), now())`,
				assignee.UserID, ticket.InternalID, ticket.RepositoryID, actorUserID,
			); err != nil {
				return err
			}
			return nil

		case "remove":
			result, err := tx.ExecContext(ctx,
				`DELETE FROM ff_ticket_assignee
				  WHERE ticket_internal_id = $1::uuid
				    AND user_id = $2`,
				ticket.InternalID, assignee.UserID,
			)
			if err != nil {
				return err
			}
			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if rowsAffected == 0 {
				return nil
			}
			return nil

		default:
			return fmt.Errorf("%w: invalid action", ErrValidation)
		}
	})
}

func (s *Store) ListTicketAssigneesForTicket(ctx context.Context, trackerSlug, ticketSlug string) ([]TicketAssignee, error) {
	trackerSlug = strings.TrimSpace(trackerSlug)
	ticketSlug = strings.TrimSpace(ticketSlug)
	if trackerSlug == "" || ticketSlug == "" {
		return []TicketAssignee{}, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT a.user_id, u.username, i.actor_id, a.created_at
		   FROM ff_ticket_assignee a
		   JOIN ff_ticket t ON t.internal_id = a.ticket_internal_id
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		   JOIN users u ON u.id = a.user_id
		   JOIN user_actor_identity i ON i.user_id = a.user_id
		  WHERE t.is_local = TRUE
		    AND tr.slug = $1
		    AND t.slug = $2
		  ORDER BY u.username ASC, a.user_id ASC`,
		trackerSlug, ticketSlug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assignees := []TicketAssignee{}
	for rows.Next() {
		var assignee TicketAssignee
		if err := rows.Scan(&assignee.UserID, &assignee.Username, &assignee.ActorID, &assignee.AssignedAt); err != nil {
			return nil, err
		}
		assignees = append(assignees, assignee)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return assignees, nil
}

func (s *Store) ListNotificationsForUser(ctx context.Context, userID int64, options NotificationListOptions) ([]Notification, bool, error) {
	if userID <= 0 {
		return []Notification{}, false, nil
	}

	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	targetCount := limit + 1
	queryLimit := targetCount * 5
	if queryLimit > 1000 {
		queryLimit = 1000
	}

	isForward := options.MinID > 0
	orderDirection := "DESC"
	if isForward {
		orderDirection = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT n.id,
		        n.kind,
		        n.unread,
		        n.created_at,
		        t.primary_key,
		        t.slug,
		        tr.slug,
		        r.slug,
		        COALESCE(u.id, 0),
		        COALESCE(u.username, ''),
		        COALESCE(i.actor_id, '')
		   FROM notification n
		   JOIN ff_ticket t ON t.internal_id = n.ticket_internal_id
		   JOIN ff_ticket_tracker tr ON tr.internal_id = t.tracker_internal_id
		   JOIN ff_repository r ON r.internal_id = n.repository_internal_id
		   LEFT JOIN users u ON u.id = n.assigned_by_user_id
		   LEFT JOIN user_actor_identity i ON i.user_id = u.id
		  WHERE n.user_id = $1
		    AND ($2::bigint <= 0 OR n.id > $2)
		    AND ($3::bigint <= 0 OR n.id < $3)
		  ORDER BY n.id %s
		  LIMIT $4`,
		orderDirection,
	)
	rows, err := s.db.QueryContext(ctx, query, userID, options.MinID, options.MaxID, queryLimit)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	notifications := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(
			&n.ID,
			&n.Type,
			&n.Unread,
			&n.CreatedAt,
			&n.TicketActorID,
			&n.TicketSlug,
			&n.TrackerSlug,
			&n.RepositorySlug,
			&n.Account.ID,
			&n.Account.Username,
			&n.Account.ActorID,
		); err != nil {
			return nil, false, err
		}
		canAccess, err := s.CanAccessRepository(ctx, n.RepositorySlug, userID)
		if err != nil {
			return nil, false, err
		}
		if !canAccess {
			continue
		}
		notifications = append(notifications, n)
		if len(notifications) >= targetCount {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(notifications) > limit
	if hasMore {
		notifications = notifications[:limit]
	}
	if isForward {
		reverseNotifications(notifications)
	}
	return notifications, hasMore, nil
}

func (s *Store) SetNotificationUnread(ctx context.Context, userID, notificationID int64, unread bool) error {
	if userID <= 0 {
		return fmt.Errorf("%w: user id is required", ErrValidation)
	}
	if notificationID <= 0 {
		return fmt.Errorf("%w: notification id is required", ErrValidation)
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE notification
		    SET unread = $1,
		        updated_at = now()
		  WHERE id = $2
		    AND user_id = $3`,
		unread, notificationID, userID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: notification not found", ErrValidation)
	}
	return nil
}

func (s *Store) SetAllNotificationsUnread(ctx context.Context, userID int64, unread bool) error {
	if userID <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE notification
		    SET unread = $1,
		        updated_at = now()
		  WHERE user_id = $2`,
		unread, userID,
	)
	return err
}

func (s *Store) CreateTicketComment(ctx context.Context, userID int64, trackerSlug, ticketSlug, content, inReplyTo string) (TicketComment, error) {
	return s.createTicketComment(ctx, userID, trackerSlug, ticketSlug, content, inReplyTo, "")
}

func (s *Store) CreateAgentTicketComment(ctx context.Context, userID int64, trackerSlug, ticketSlug, content, inReplyTo, agentCommentKind string) (TicketComment, error) {
	return s.createTicketComment(ctx, userID, trackerSlug, ticketSlug, content, inReplyTo, agentCommentKind)
}

func (s *Store) createTicketComment(ctx context.Context, userID int64, trackerSlug, ticketSlug, content, inReplyTo, agentCommentKind string) (TicketComment, error) {
	content = strings.TrimSpace(content)
	inReplyTo = strings.TrimSpace(inReplyTo)
	agentCommentKind = strings.TrimSpace(agentCommentKind)
	if content == "" {
		return TicketComment{}, fmt.Errorf("%w: content is required", ErrValidation)
	}
	if agentCommentKind != "" && agentCommentKind != AgentCommentKindAck && agentCommentKind != AgentCommentKindCompletion {
		return TicketComment{}, fmt.Errorf("%w: invalid agent comment kind", ErrValidation)
	}

	var comment TicketComment
	err := withTx(s.db, ctx, func(tx *sql.Tx) error {
		ticket, err := loadTicketRowBySlugsTx(ctx, tx, trackerSlug, ticketSlug)
		if err != nil {
			return err
		}
		canAccess, err := canAccessRepositoryTx(ctx, tx, ticket.RepositoryID, userID)
		if err != nil {
			return err
		}
		if !canAccess {
			return fmt.Errorf("%w: repository access denied", ErrValidation)
		}

		ticketBody, err := parseBody(ticket.Body)
		if err != nil {
			return err
		}
		recipientActorID := stringFromAny(ticketBody["target"])
		if recipientActorID == "" {
			return fmt.Errorf("%w: ticket recipient is missing", ErrValidation)
		}

		var authorActorID string
		if err := tx.QueryRowContext(ctx,
			`SELECT actor_id FROM user_actor_identity WHERE user_id = $1`,
			userID,
		).Scan(&authorActorID); err != nil {
			return err
		}

		parentActorID := ticket.ActorID
		if inReplyTo != "" && inReplyTo != ticket.ActorID {
			parent, err := loadTicketCommentRowByActorIDTx(ctx, tx, inReplyTo)
			if err != nil {
				return err
			}
			if parent.TicketInternalID != ticket.InternalID {
				return fmt.Errorf("%w: parent comment belongs to a different ticket", ErrValidation)
			}
			if parent.RecipientActorID != recipientActorID {
				return fmt.Errorf("%w: parent comment recipient mismatch", ErrValidation)
			}
			parentActorID = parent.NotePrimaryKey
		} else if inReplyTo == ticket.ActorID {
			parentActorID = ticket.ActorID
		}

		commentSlug := NewTicketID()
		commentInternalID := uuid.New()
		commentActorID := s.BaseURL() + "/ticket-tracker/" + ticket.TrackerSlug + "/ticket/" + ticket.Slug + "/comment/" + commentSlug
		published := time.Now().UTC()
		commentBody := buildLocalTicketCommentObject(
			commentActorID,
			ticket.ActorID,
			parentActorID,
			authorActorID,
			recipientActorID,
			content,
			agentCommentKind,
			published,
		)
		if _, err := upsertObjectTx(ctx, tx, commentActorID, "Note", commentBody); err != nil {
			return err
		}

		if err := tx.QueryRowContext(ctx,
			`INSERT INTO ff_ticket_comment (
				internal_id,
				slug,
				note_primary_key,
				ticket_internal_id,
				repository_internal_id,
				in_reply_to_note_primary_key,
				created_by_user_id,
				recipient_actor_id,
				is_local,
				created_at,
				updated_at
			) VALUES (
				$1::uuid, $2, $3, $4::uuid, $5::uuid, NULLIF($6, ''), $7, $8, TRUE, now(), now()
			)
			RETURNING id`,
			commentInternalID.String(),
			commentSlug,
			commentActorID,
			ticket.InternalID,
			ticket.RepositoryID,
			nullString(parentActorID, ticket.ActorID),
			userID,
			recipientActorID,
		).Scan(&comment.ID); err != nil {
			return err
		}

		comment.InternalID = commentInternalID.String()
		comment.Slug = commentSlug
		comment.ActorID = commentActorID
		comment.TicketSlug = ticket.Slug
		comment.TrackerSlug = ticket.TrackerSlug
		comment.RepositorySlug = ticket.RepositorySlug
		comment.InReplyToActorID = parentActorID
		comment.InReplyToTicketID = parentActorID == ticket.ActorID
		comment.AttributedTo = authorActorID
		comment.Content = content
		comment.Published = published.Format(time.RFC3339Nano)
		comment.RecipientActorID = recipientActorID
		return nil
	})
	if err != nil {
		return TicketComment{}, err
	}
	return comment, nil
}

func (s *Store) CreateTicketUpdate(ctx context.Context, userID int64, trackerSlug, ticketSlug, content string) (UpdateRecord, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return UpdateRecord{}, fmt.Errorf("%w: content is required", ErrValidation)
	}

	var out UpdateRecord
	err := withTx(s.db, ctx, func(tx *sql.Tx) error {
		ticket, err := loadTicketRowBySlugsTx(ctx, tx, trackerSlug, ticketSlug)
		if err != nil {
			return err
		}
		canAccess, err := canAccessRepositoryTx(ctx, tx, ticket.RepositoryID, userID)
		if err != nil {
			return err
		}
		if !canAccess {
			return fmt.Errorf("%w: repository access denied", ErrValidation)
		}

		authorActorID, err := loadUserActorIDTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		published := time.Now().UTC()
		updateID := s.BaseURL() + "/update/" + NewTicketID()

		if err := snapshotObjectVersionTx(ctx, tx, ticket.ActorID, "Ticket", ticket.Body, updateID, userID); err != nil {
			return err
		}
		updatedBodyRaw, err := appendPlainTextUpdate(ticket.Body, content, published)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE ff_ticket
			    SET body = $1::jsonb,
			        updated_at = now()
			  WHERE internal_id = $2::uuid`,
			string(updatedBodyRaw),
			ticket.InternalID,
		); err != nil {
			return err
		}

		updateBody := buildLocalUpdateObject(updateID, authorActorID, ticket.ActorID, content, published)
		if _, err := upsertObjectTx(ctx, tx, updateID, "Update", updateBody); err != nil {
			return err
		}

		out = UpdateRecord{
			PrimaryKey:       updateID,
			ObjectPrimaryKey: ticket.ActorID,
			ObjectType:       "Ticket",
			Content:          content,
			Published:        published,
		}
		return nil
	})
	if err != nil {
		return UpdateRecord{}, err
	}
	return out, nil
}

func (s *Store) CreateTicketCommentUpdate(ctx context.Context, userID int64, trackerSlug, ticketSlug, commentSlug, content string) (UpdateRecord, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return UpdateRecord{}, fmt.Errorf("%w: content is required", ErrValidation)
	}

	var out UpdateRecord
	err := withTx(s.db, ctx, func(tx *sql.Tx) error {
		comment, err := loadTicketCommentRowBySlugsTx(ctx, tx, trackerSlug, ticketSlug, commentSlug)
		if err != nil {
			return err
		}
		canAccess, err := canAccessRepositoryTx(ctx, tx, comment.RepositoryInternalID, userID)
		if err != nil {
			return err
		}
		if !canAccess {
			return fmt.Errorf("%w: repository access denied", ErrValidation)
		}

		authorActorID, err := loadUserActorIDTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		published := time.Now().UTC()
		updateID := s.BaseURL() + "/update/" + NewTicketID()

		if err := snapshotObjectVersionTx(ctx, tx, comment.NotePrimaryKey, "Note", comment.Body, updateID, userID); err != nil {
			return err
		}
		updatedBodyRaw, err := appendPlainTextUpdate(comment.Body, content, published)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE as_note
			    SET body = $1::jsonb,
			        updated_at = now()
			  WHERE primary_key = $2`,
			string(updatedBodyRaw),
			comment.NotePrimaryKey,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE ff_ticket_comment
			    SET updated_at = now()
			  WHERE internal_id = $1::uuid`,
			comment.InternalID,
		); err != nil {
			return err
		}

		updateBody := buildLocalUpdateObject(updateID, authorActorID, comment.NotePrimaryKey, content, published)
		if _, err := upsertObjectTx(ctx, tx, updateID, "Update", updateBody); err != nil {
			return err
		}

		out = UpdateRecord{
			PrimaryKey:       updateID,
			ObjectPrimaryKey: comment.NotePrimaryKey,
			ObjectType:       "Note",
			Content:          content,
			Published:        published,
		}
		return nil
	})
	if err != nil {
		return UpdateRecord{}, err
	}
	return out, nil
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

func (s *Store) listObjectVersionsByPrimaryKey(ctx context.Context, objectPrimaryKey string, limit int) ([]ObjectVersionRecord, error) {
	objectPrimaryKey = strings.TrimSpace(objectPrimaryKey)
	if objectPrimaryKey == "" {
		return []ObjectVersionRecord{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id,
		        object_primary_key,
		        object_type,
		        body,
		        COALESCE(source_update_primary_key, ''),
		        COALESCE(created_by_user_id, 0),
		        created_at
		   FROM ap_object_version
		  WHERE object_primary_key = $1
		  ORDER BY created_at DESC, id DESC
		  LIMIT $2`,
		objectPrimaryKey,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := []ObjectVersionRecord{}
	for rows.Next() {
		var item ObjectVersionRecord
		if err := rows.Scan(
			&item.ID,
			&item.ObjectPrimaryKey,
			&item.ObjectType,
			&item.BodyJSON,
			&item.SourceUpdatePrimaryKey,
			&item.CreatedByUserID,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		versions = append(versions, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return versions, nil
}

func (s *Store) ListObjectTypeCounts(ctx context.Context) ([]ObjectTypeCount, error) {
	counts := make([]ObjectTypeCount, 0, len(objectTables))
	for _, table := range objectTables {
		var count int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		counts = append(counts, ObjectTypeCount{Table: table, Count: count})
	}
	sort.Slice(counts, func(i, j int) bool {
		return counts[i].Table < counts[j].Table
	})
	return counts, nil
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

func (s *Store) ensureUserActorIdentityBackfill(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username
	       FROM users
	      WHERE id NOT IN (SELECT user_id FROM user_actor_identity)
	      ORDER BY id`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type missingUser struct {
		id       int64
		username string
	}
	var missing []missingUser
	for rows.Next() {
		var u missingUser
		if err := rows.Scan(&u.id, &u.username); err != nil {
			return err
		}
		missing = append(missing, u)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, u := range missing {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := provisionUserActorTx(ctx, tx, u.id, u.username, s.BaseURL()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	slog.Debug("user actor backfill complete", "count", len(missing))
	return nil
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
