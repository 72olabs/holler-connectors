package bus

import "time"

// Room visibility. A public named room can be listed, joined and left by any
// local agent without approval; a private one only admits members a manager
// adds or approves. DMs are always private.
const (
	VisibilityPrivate = "private"
	VisibilityPublic  = "public"
)

// Join request states. Only open requests can be decided or withdrawn.
const (
	JoinRequestOpen      = "open"
	JoinRequestApproved  = "approved"
	JoinRequestDeclined  = "declined"
	JoinRequestWithdrawn = "withdrawn"
	JoinRequestExpired   = "expired"
)

// RoomSummary is what anyone may see about a named room without a grant:
// metadata only, never messages or activity.
type RoomSummary struct {
	ChannelID    string       `json:"channel_id"`
	ProjectID    string       `json:"project_id"`
	Title        string       `json:"title"`
	Purpose      string       `json:"purpose"`
	Creator      string       `json:"created_by"`
	Participants []string     `json:"participants"`
	Visibility   string       `json:"visibility"`
	Member       bool         `json:"member"`
	CanManage    bool         `json:"can_manage"`
	MyRequest    *JoinRequest `json:"my_request,omitempty"`
}

type RoomDirectory struct {
	Rooms      []RoomSummary `json:"rooms"`
	NextCursor string        `json:"next_cursor"`
}

// JoinRequest asks a room's managers to admit the requester. It grants
// nothing until a manager approves it.
type JoinRequest struct {
	ID           string     `json:"request_id"`
	ChannelID    string     `json:"channel_id"`
	ChannelTitle string     `json:"channel_title"`
	Requester    string     `json:"requester"`
	Reason       string     `json:"reason"`
	State        string     `json:"state"`
	DecidedBy    string     `json:"decided_by,omitempty"`
	Note         string     `json:"note,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
	ExpiresAt    time.Time  `json:"expires_at"`
}

// JoinRequestList is one page, newest first. next_cursor is empty on the last.
type JoinRequestList struct {
	Requests   []JoinRequest `json:"requests"`
	NextCursor string        `json:"next_cursor"`
}

type ChannelDiscover struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type ChannelJoin struct {
	ChannelID      string `json:"channel_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ChannelLeave struct {
	ChannelID      string `json:"channel_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ChannelLeft struct {
	ChannelID string `json:"channel_id"`
	Left      bool   `json:"left"`
}

type JoinRequestCreate struct {
	ChannelID      string `json:"channel_id"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

type JoinRequestWithdraw struct {
	RequestID      string `json:"request_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type JoinRequestDecide struct {
	RequestID        string `json:"request_id"`
	Decision         string `json:"decision"`
	Note             string `json:"note,omitempty"`
	ExpectedRevision int64  `json:"expected_policy_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type JoinRequestQuery struct {
	State  string `json:"state,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
