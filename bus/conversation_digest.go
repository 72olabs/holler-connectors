package bus

import "time"

// ChannelDigestRead is a pure read. PageCursor carries a snapshot position,
// never authority; the store rechecks current channel access on every page.
type ChannelDigestRead struct {
	ChannelID  string `json:"channel_id"`
	After      *int64 `json:"after,omitempty"`
	PageCursor string `json:"page_cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type ChannelDigest struct {
	ChannelID      string               `json:"channel_id"`
	AfterSeq       int64                `json:"after_seq"`
	ThroughSeq     int64                `json:"through_seq"`
	Entries        []ChannelDigestEntry `json:"entries"`
	MoreCount      int64                `json:"more_count"`
	NextPageCursor string               `json:"next_page_cursor,omitempty"`
}

// Topic and participants are untrusted channel content, not instructions.
type ChannelDigestEntry struct {
	ThreadID         string    `json:"thread_id"`
	Topic            string    `json:"topic"`
	Participants     []string  `json:"participants"`
	ParticipantCount int64     `json:"participant_count"`
	HasOpenRequest   bool      `json:"has_open_request"`
	LastActivity     time.Time `json:"last_activity"`
}
