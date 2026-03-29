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
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"golang.org/x/crypto/argon2"
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

func fakeHashWork(password string) {
	salt := make([]byte, defaultParams.saltLen)
	_, _ = rand.Read(salt)
	_ = argon2.IDKey([]byte(password), salt, defaultParams.time, defaultParams.memory, defaultParams.threads, defaultParams.keyLen)
}

// UserStore owns all user management and authentication queries.
type UserStore struct {
	db      *sql.DB
	baseURL string
}

func (s *UserStore) CreateUser(ctx context.Context, username, password string) error {
	return s.CreateUserWithAdmin(ctx, username, password, false)
}

func (s *UserStore) CreateUserWithAdmin(ctx context.Context, username, password string, isAdmin bool) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("%w: empty username", ErrValidation)
	}
	if len(password) < 12 {
		return fmt.Errorf("%w: password too short (min 12 chars)", ErrValidation)
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

	if err := provisionUserActorTx(ctx, tx, userID, username, s.baseURL); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *UserStore) DeleteUser(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("%w: username cannot be empty", ErrValidation)
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE username = $1`, username)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: user not found", ErrValidation)
	}
	return nil
}

func (s *UserStore) IsUserAdmin(ctx context.Context, userID int64) (bool, error) {
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

func (s *UserStore) VerifyUser(ctx context.Context, username, password string) (int64, bool, error) {
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

func (s *UserStore) GetUsernameByID(ctx context.Context, userID int64) (string, bool, error) {
	var username string
	err := s.db.QueryRowContext(ctx,
		`SELECT username FROM users WHERE id = $1`,
		userID,
	).Scan(&username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return username, true, nil
}

func (s *UserStore) GetUserProfileByID(ctx context.Context, userID int64) (UserProfileRecord, bool, error) {
	var r UserProfileRecord
	err := s.db.QueryRowContext(ctx,
		`SELECT is_admin, created_at FROM users WHERE id = $1`,
		userID,
	).Scan(&r.IsAdmin, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	return r, true, nil
}

func (s *UserStore) GetUserActorByUsername(ctx context.Context, username string) (UserActorRecord, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return UserActorRecord{}, false, nil
	}

	var record UserActorRecord
	var actorRaw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, i.actor_id, i.main_key_id, p.body
	       FROM users u
	       JOIN user_actor_identity i ON i.user_id = u.id
	       JOIN as_person p ON p.primary_key = i.actor_id
	      WHERE u.username = $1`,
		username,
	).Scan(&record.UserID, &record.Username, &record.ActorID, &record.MainKeyID, &actorRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserActorRecord{}, false, nil
		}
		return UserActorRecord{}, false, err
	}

	var obj map[string]any
	if err := json.Unmarshal(actorRaw, &obj); err != nil {
		return UserActorRecord{}, false, fmt.Errorf("invalid stored actor json: %w", err)
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return UserActorRecord{}, false, err
	}
	record.ActorJSON = raw
	return record, true, nil
}

func (s *UserStore) ensureUserActorIdentityBackfill(ctx context.Context) error {
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
		if err := provisionUserActorTx(ctx, tx, u.id, u.username, s.baseURL); err != nil {
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

