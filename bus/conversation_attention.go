package bus

import (
	"errors"
	"time"
)

type AttentionMode string

const (
	AttentionChannel  AttentionMode = "channel"
	AttentionThreaded AttentionMode = "threaded"
	// This affects only new admissions without an explicit agent default.
	// Upgraded memberships are always snapshotted as Channel independently.
	// Founder decision: human:owner msg_5bc043f043035f072c42fcd878162604.
	DefaultAttentionMode = AttentionThreaded
)

var (
	ErrAttentionConflict = errors.New("attention settings changed; refresh before retrying")
	ErrAttentionLocked   = errors.New("attention mode is locked by the owner")
	ErrThreadObligation  = errors.New("cannot unfollow a thread with an open obligation")
)

type ThreadFollowChange struct {
	ChannelID        string `json:"channel_id"`
	ThreadID         string `json:"thread_id"`
	Follow           bool   `json:"follow"`
	ExpectedRevision int64  `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type ThreadFollowState struct {
	ChannelID string `json:"channel_id"`
	ThreadID  string `json:"thread_id"`
	Actor     string `json:"actor"`
	Following bool   `json:"following"`
	Reason    string `json:"reason"`
	Revision  int64  `json:"revision"`
	SinceSeq  int64  `json:"since_seq"`
}

type ChannelAttentionChange struct {
	ChannelID        string        `json:"channel_id"`
	Mode             AttentionMode `json:"mode"`
	Until            *time.Time    `json:"until,omitempty"`
	ExpectedRevision int64         `json:"expected_revision"`
	IdempotencyKey   string        `json:"idempotency_key"`
	// Owner-only. Narrow agent tools must never expose these fields. The store
	// checks trusted transport authority and current access, never role text.
	TargetActor string `json:"target_actor,omitempty"`
	Locked      *bool  `json:"locked,omitempty"`
}

type ChannelMemberAttention struct {
	ChannelID string        `json:"channel_id"`
	Actor     string        `json:"actor"`
	Mode      AttentionMode `json:"mode"`
	BaseMode  AttentionMode `json:"base_mode"`
	Until     *time.Time    `json:"until,omitempty"`
	LockedBy  string        `json:"locked_by,omitempty"`
	Revision  int64         `json:"revision"`
}

type AgentAttentionDefaultChange struct {
	Mode             AttentionMode `json:"mode"`
	ExpectedRevision int64         `json:"expected_revision"`
	IdempotencyKey   string        `json:"idempotency_key"`
}

type AgentAttentionDefault struct {
	Actor    string        `json:"actor"`
	Mode     AttentionMode `json:"mode"`
	Revision int64         `json:"revision"`
}

type ChannelDigestMark struct {
	ChannelID        string `json:"channel_id"`
	ThroughSeq       int64  `json:"through_seq"`
	ExpectedRevision int64  `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type ChannelDigestCheckpoint struct {
	ChannelID   string `json:"channel_id"`
	LastSeenSeq int64  `json:"last_seen_seq"`
	Revision    int64  `json:"revision"`
}
