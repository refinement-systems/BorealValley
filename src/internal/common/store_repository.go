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
	"path/filepath"
	"strings"
)

// RepositoryStore owns all repository and membership queries.
type RepositoryStore struct {
	db      *sql.DB
	baseURL string
	rootDir string
}

func (s *RepositoryStore) ListRepositories(ctx context.Context) ([]Repository, error) {
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

func (s *RepositoryStore) GetRepositoryBySlug(ctx context.Context, repoSlug string) (Repository, bool, error) {
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

func (s *RepositoryStore) ListRepositoryMembers(ctx context.Context, repoSlug string) ([]RepositoryMember, error) {
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

func (s *RepositoryStore) AddRepositoryMemberByUsername(ctx context.Context, repoSlug, username string) error {
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

func (s *RepositoryStore) RemoveRepositoryMemberByUsername(ctx context.Context, repoSlug, username string) error {
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

func (s *RepositoryStore) IsRepositoryMember(ctx context.Context, repoSlug string, userID int64) (bool, error) {
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

func (s *RepositoryStore) CanAccessRepository(ctx context.Context, repoSlug string, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	var isAdmin bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT is_admin FROM users WHERE id = $1`,
		userID,
	).Scan(&isAdmin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if isAdmin {
		return true, nil
	}
	return s.IsRepositoryMember(ctx, repoSlug, userID)
}

func (s *RepositoryStore) ResyncFromFilesystem(ctx context.Context) error {
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
		if !IsPijulRepo(repoDir) {
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

		repoActorID := s.baseURL + "/repo/" + repoSlug
		repoBody := buildLocalRepositoryObject(repoActorID, entry.Name())
		if err := upsertLocalRepositoryTx(ctx, tx, repoActorID, repoBody, repoID, repoSlug, repoDir); err != nil {
			return err
		}
	}

	return tx.Commit()
}
