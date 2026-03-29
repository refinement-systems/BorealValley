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

import "time"

const (
	PostgresDSNEnv             = "BV_PG_DSN"
	AgentCommentKindField      = "borealValleyAgentCommentKind"
	AgentCommentKindAck        = "ack"
	AgentCommentKindCompletion = "completion"
)

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
