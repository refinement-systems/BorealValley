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
	"fmt"
)

// NotificationStore owns all notification queries.
type NotificationStore struct {
	db *sql.DB
}

func (s *NotificationStore) ListNotificationsForUser(ctx context.Context, userID int64, options NotificationListOptions) ([]Notification, bool, error) {
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

	isForward := options.MinID > 0

	var query string
	if isForward {
		query = `SELECT n.id,
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
		    AND (
		        EXISTS(SELECT 1 FROM users WHERE id = $1 AND is_admin = TRUE)
		        OR EXISTS(
		            SELECT 1
		              FROM ff_repository_member m
		             WHERE m.repository_internal_id = r.internal_id
		               AND m.user_id = $1
		               AND r.is_local = TRUE
		        )
		    )
		  ORDER BY n.id ASC
		  LIMIT $4`
	} else {
		query = `SELECT n.id,
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
		    AND (
		        EXISTS(SELECT 1 FROM users WHERE id = $1 AND is_admin = TRUE)
		        OR EXISTS(
		            SELECT 1
		              FROM ff_repository_member m
		             WHERE m.repository_internal_id = r.internal_id
		               AND m.user_id = $1
		               AND r.is_local = TRUE
		        )
		    )
		  ORDER BY n.id DESC
		  LIMIT $4`
	}
	rows, err := s.db.QueryContext(ctx, query, userID, options.MinID, options.MaxID, targetCount)
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
		notifications = append(notifications, n)
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

func (s *NotificationStore) SetNotificationUnread(ctx context.Context, userID, notificationID int64, unread bool) error {
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

func (s *NotificationStore) SetAllNotificationsUnread(ctx context.Context, userID int64, unread bool) error {
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
