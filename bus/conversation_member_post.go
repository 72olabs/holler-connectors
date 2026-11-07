package bus

import (
	"encoding/json"
	"errors"
)

// The member post lane (C5): a plain post an agent may make without a
// separate approval, only where the daemon allows it. Everything it refuses
// still goes through the explicitly approved channel.post.
var (
	ErrMemberPostNotEligible = errors.New("this post needs the user's approval: ask them, then use holler_write channel.post")
	ErrHumanTurnRequired     = errors.New("wait for a human message in this channel, or ask the human one decision with respondent set to them")
	ErrTooManyOpenRequests   = errors.New("you already have three open questions for this person; wait for their answers")
)

// ChannelMemberPost carries only what a plain post needs. No references,
// work proposals or confirmations, authorization sources or answers: a strict
// decode refuses those fields, so they stay behind channel.post.
type ChannelMemberPost struct {
	ChannelID        string          `json:"channel_id"`
	ThreadID         string          `json:"thread_id,omitempty"`
	InReplyTo        string          `json:"in_reply_to,omitempty"`
	ExpectedRevision int64           `json:"expected_policy_revision"`
	IdempotencyKey   string          `json:"idempotency_key"`
	Body             json.RawMessage `json:"body"`
	Attention        []string        `json:"attention_targets,omitempty"`
	Respondent       string          `json:"respondent,omitempty"`
}

// ChannelMemberPostSetting turns the lane on or off for one channel. Only a
// person who can post there may change it; it is on until they do.
type ChannelMemberPostSetting struct {
	ChannelID        string `json:"channel_id"`
	Enabled          bool   `json:"enabled"`
	ExpectedRevision int64  `json:"expected_policy_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}
