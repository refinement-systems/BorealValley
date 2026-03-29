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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

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

func upsertLocalRepositoryTx(ctx context.Context, tx *sql.Tx, actorID string, body any, internalID uuid.UUID, slug, fsPath string) error {
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
