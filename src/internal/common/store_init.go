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
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/refinement-systems/BorealValley/src/internal/assets"
)

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
		query, ok := objectTableCountQueries[table]
		if !ok {
			return nil, fmt.Errorf("disallowed table name: %q", table)
		}
		var count int64
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
