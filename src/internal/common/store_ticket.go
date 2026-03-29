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
	"time"

	"github.com/google/uuid"
)

// TicketStore owns all ticket tracker, ticket, and comment queries.
type TicketStore struct {
	db      *sql.DB
	baseURL string
}

func (s *TicketStore) ListTicketTrackers(ctx context.Context) ([]TicketTracker, error) {
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

func (s *TicketStore) GetTicketTrackerBySlug(ctx context.Context, trackerSlug string) (TicketTracker, bool, error) {
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

func (s *TicketStore) ListRepositoriesForTracker(ctx context.Context, trackerSlug string) ([]Repository, error) {
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

func (s *TicketStore) CreateTicketTracker(ctx context.Context, userID int64, name, summary string) (TicketTracker, error) {
	name = strings.TrimSpace(name)
	summary = strings.TrimSpace(summary)
	if name == "" {
		return TicketTracker{}, fmt.Errorf("%w: name is required", ErrValidation)
	}

	slug := slugify(name)
	internalID := uuid.New()
	actorID := s.baseURL + "/ticket-tracker/" + slug
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

func (s *TicketStore) AssignTicketTrackerToRepository(ctx context.Context, repoSlug, trackerSlug string) error {
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

func (s *TicketStore) UnassignTicketTrackerFromRepository(ctx context.Context, repoSlug string) error {
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
			  WHERE internal_id = $1::uuid`,
			repo.InternalID,
		); err != nil {
			return err
		}
		return nil
	})
}

func (s *TicketStore) CreateTicket(ctx context.Context, userID int64, trackerSlug, repoSlug, summary, content string) (Ticket, error) {
	return s.CreateTicketWithPriority(ctx, userID, trackerSlug, repoSlug, summary, content, 0)
}

func (s *TicketStore) CreateTicketWithPriority(ctx context.Context, userID int64, trackerSlug, repoSlug, summary, content string, priority int) (Ticket, error) {
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
		ticketActorID := s.baseURL + "/ticket-tracker/" + tracker.Slug + "/ticket/" + ticketSlug
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

func (s *TicketStore) UpdateTicketAssigneeByUsername(ctx context.Context, actorUserID int64, trackerSlug, ticketSlug, action, username string) error {
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

func (s *TicketStore) ListTicketAssigneesForTicket(ctx context.Context, trackerSlug, ticketSlug string) ([]TicketAssignee, error) {
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

func (s *TicketStore) CreateTicketComment(ctx context.Context, userID int64, trackerSlug, ticketSlug, content, inReplyTo string) (TicketComment, error) {
	return s.createTicketComment(ctx, userID, trackerSlug, ticketSlug, content, inReplyTo, "")
}

func (s *TicketStore) CreateAgentTicketComment(ctx context.Context, userID int64, trackerSlug, ticketSlug, content, inReplyTo, agentCommentKind string) (TicketComment, error) {
	return s.createTicketComment(ctx, userID, trackerSlug, ticketSlug, content, inReplyTo, agentCommentKind)
}

func (s *TicketStore) createTicketComment(ctx context.Context, userID int64, trackerSlug, ticketSlug, content, inReplyTo, agentCommentKind string) (TicketComment, error) {
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
		commentActorID := s.baseURL + "/ticket-tracker/" + ticket.TrackerSlug + "/ticket/" + ticket.Slug + "/comment/" + commentSlug
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

func (s *TicketStore) CreateTicketUpdate(ctx context.Context, userID int64, trackerSlug, ticketSlug, content string) (UpdateRecord, error) {
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
		updateID := s.baseURL + "/update/" + NewTicketID()

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

func (s *TicketStore) CreateTicketCommentUpdate(ctx context.Context, userID int64, trackerSlug, ticketSlug, commentSlug, content string) (UpdateRecord, error) {
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
		updateID := s.baseURL + "/update/" + NewTicketID()

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

func (s *TicketStore) listObjectVersionsByPrimaryKey(ctx context.Context, objectPrimaryKey string, limit int) ([]ObjectVersionRecord, error) {
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

func (s *TicketStore) ListTickets(ctx context.Context) ([]Ticket, error) {
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

func (s *TicketStore) ListTicketsForTracker(ctx context.Context, trackerSlug string) ([]Ticket, error) {
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

func (s *TicketStore) GetLocalRepositoryObjectBySlug(ctx context.Context, repoSlug string) (LocalObjectRecord, bool, error) {
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

func (s *TicketStore) GetLocalTicketTrackerObjectBySlug(ctx context.Context, trackerSlug string) (LocalObjectRecord, bool, error) {
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

func (s *TicketStore) GetLocalTicketObjectBySlug(ctx context.Context, trackerSlug, ticketSlug string) (LocalTicketObjectRecord, bool, error) {
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

func (s *TicketStore) GetLocalTicketCommentObjectBySlug(ctx context.Context, trackerSlug, ticketSlug, commentSlug string) (LocalTicketCommentObjectRecord, bool, error) {
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
