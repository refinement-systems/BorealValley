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
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type UserActorRecord struct {
	UserID    int64
	Username  string
	ActorID   string
	MainKeyID string
	ActorJSON []byte
}

func provisionUserActorTx(ctx context.Context, tx *sql.Tx, userID int64, username string, baseURL string) error {
	var existingActorID string
	err := tx.QueryRowContext(ctx,
		`SELECT actor_id FROM user_actor_identity WHERE user_id = $1`,
		userID,
	).Scan(&existingActorID)
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}

	actor, privateKeyMultibase, mainKeyID, err := buildUserActorForUsername(username, baseURL)
	if err != nil {
		return err
	}

	if _, err := upsertObjectTx(ctx, tx, actor.ID, actor.Type, actor.RawObject); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_actor_identity (user_id, actor_id, main_key_id, private_key_multibase, created_at)
	         VALUES ($1, $2, $3, $4, now())
	      ON CONFLICT (user_id) DO UPDATE SET
	         actor_id = EXCLUDED.actor_id,
	         main_key_id = EXCLUDED.main_key_id,
	         private_key_multibase = EXCLUDED.private_key_multibase`,
		userID, actor.ID, mainKeyID, privateKeyMultibase,
	)
	return err
}

type localActor struct {
	ID        string
	Type      string
	RawObject map[string]any
}

func buildUserActorForUsername(username string, baseURL string) (localActor, string, string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return localActor{}, "", "", errors.New("empty username")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return localActor{}, "", "", errors.New("empty base url")
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return localActor{}, "", "", err
	}

	publicKeyMultibase := ed25519PublicKeyToMultibase(publicKey)
	privateKeyMultibase := "z" + base58EncodeRaw(privateKey)

	actorID := baseURL + "/users/" + username
	mainKeyID := actorID + "#main-key"
	actorPayload := map[string]any{
		"@context":          []string{"https://www.w3.org/ns/activitystreams", "https://forgefed.org/ns"},
		"id":                actorID,
		"type":              "Person",
		"inbox":             actorID + "/inbox",
		"outbox":            actorID + "/outbox",
		"preferredUsername": username,
		"assertionMethod": []map[string]any{
			{
				"id":                 mainKeyID,
				"type":               "Multikey",
				"controller":         actorID,
				"publicKeyMultibase": publicKeyMultibase,
			},
		},
	}

	return localActor{
		ID:        actorID,
		Type:      "Person",
		RawObject: actorPayload,
	}, privateKeyMultibase, mainKeyID, nil
}

func ed25519PublicKeyToMultibase(publicKey ed25519.PublicKey) string {
	buf := make([]byte, 0, 2+len(publicKey))
	buf = append(buf, 0xed, 0x01)
	buf = append(buf, publicKey...)
	return "z" + base58EncodeRaw(buf)
}

func base58EncodeRaw(input []byte) string {
	if len(input) == 0 {
		return ""
	}

	zeros := 0
	for zeros < len(input) && input[zeros] == 0 {
		zeros++
	}

	buf := make([]byte, len(input))
	copy(buf, input)
	out := make([]byte, 0, len(input)*2)
	for start := zeros; start < len(buf); {
		remainder := 0
		for i := start; i < len(buf); i++ {
			value := int(buf[i]) + remainder*256
			buf[i] = byte(value / 58)
			remainder = value % 58
		}
		out = append(out, base58Alphabet[remainder])
		for start < len(buf) && buf[start] == 0 {
			start++
		}
	}

	for i := 0; i < zeros; i++ {
		out = append(out, base58Alphabet[0])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func (s *Store) GetUsernameByID(ctx context.Context, userID int64) (string, bool, error) {
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

func (s *Store) GetUserActorByUsername(ctx context.Context, username string) (UserActorRecord, bool, error) {
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

	// Force canonical JSON formatting for deterministic responses.
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
