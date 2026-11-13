package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/72olabs/holler-connectors/bus"
)

type attentionCAS struct {
	ChannelID        string `json:"channel_id"`
	ExpectedRevision *int64 `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}

func (a attentionCAS) validate() error {
	if a.ExpectedRevision == nil || *a.ExpectedRevision < 0 {
		return &bus.ValidationError{Field: "expected_revision", Problem: "a nonnegative revision is required"}
	}
	for field, value := range map[string]string{"channel_id": a.ChannelID, "idempotency_key": a.IdempotencyKey} {
		if strings.TrimSpace(value) == "" || bus.ValidateTextIdentifier(field, value, 256) != nil {
			return &bus.ValidationError{Field: field, Problem: "a bounded identifier is required"}
		}
	}
	return nil
}

// These fixed writes affect only the bound actor's attention/checkpoint. Owner
// target and lock selectors deliberately do not appear in any narrow schema.
func (s *Server) changeAttention(ctx context.Context, name string, raw json.RawMessage) (interface{}, error) {
	bridge, ok := s.store.(capabilityStore)
	if !ok {
		return nil, bus.ErrChannelCapability
	}
	var capability string
	var request interface{}
	switch name {
	case "holler_thread_follow", "holler_thread_unfollow":
		var a struct {
			attentionCAS
			ThreadID string `json:"thread_id"`
		}
		if err := decodeStrict(raw, &a); err != nil {
			return nil, err
		}
		if err := a.validate(); err != nil {
			return nil, err
		}
		if strings.TrimSpace(a.ThreadID) == "" || bus.ValidateTextIdentifier("thread_id", a.ThreadID, 256) != nil {
			return nil, &bus.ValidationError{Field: "thread_id", Problem: "a bounded thread identifier is required"}
		}
		capability = "channel.thread.follow"
		request = bus.ThreadFollowChange{ChannelID: a.ChannelID, ThreadID: a.ThreadID, Follow: name == "holler_thread_follow", ExpectedRevision: *a.ExpectedRevision, IdempotencyKey: a.IdempotencyKey}
	case "holler_channel_set_mode":
		var a struct {
			attentionCAS
			Mode  bus.AttentionMode `json:"mode"`
			Until *time.Time        `json:"until,omitempty"`
		}
		if err := decodeStrict(raw, &a); err != nil {
			return nil, err
		}
		if err := a.validate(); err != nil {
			return nil, err
		}
		if a.Mode != bus.AttentionChannel && a.Mode != bus.AttentionThreaded {
			return nil, &bus.ValidationError{Field: "mode", Problem: "must be channel or threaded; delegated is unavailable"}
		}
		capability = "channel.attention.set"
		request = bus.ChannelAttentionChange{ChannelID: a.ChannelID, Mode: a.Mode, Until: a.Until, ExpectedRevision: *a.ExpectedRevision, IdempotencyKey: a.IdempotencyKey}
	case "holler_channel_digest_mark":
		var a struct {
			attentionCAS
			ThroughSeq *int64 `json:"through_seq"`
		}
		if err := decodeStrict(raw, &a); err != nil {
			return nil, err
		}
		if err := a.validate(); err != nil {
			return nil, err
		}
		if a.ThroughSeq == nil || *a.ThroughSeq < 0 {
			return nil, &bus.ValidationError{Field: "through_seq", Problem: "the nonnegative frozen digest boundary is required"}
		}
		capability = "channel.digest_mark"
		request = bus.ChannelDigestMark{ChannelID: a.ChannelID, ThroughSeq: *a.ThroughSeq, ExpectedRevision: *a.ExpectedRevision, IdempotencyKey: a.IdempotencyKey}
	default:
		return nil, bus.ErrChannelCapability
	}
	args, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	return bridge.InvokeWriteCapability(ctx, capability, args)
}

func attentionToolDefinitions() []map[string]interface{} {
	tool := func(name, description string, props map[string]interface{}, extra string) map[string]interface{} {
		props["channel_id"] = map[string]interface{}{"type": "string"}
		props["expected_revision"] = map[string]interface{}{"type": "integer", "minimum": 0}
		props["idempotency_key"] = map[string]interface{}{"type": "string"}
		return map[string]interface{}{
			"name": name, "description": description,
			"inputSchema": map[string]interface{}{"type": "object", "properties": props, "additionalProperties": false, "required": []string{"channel_id", extra, "expected_revision", "idempotency_key"}},
			"annotations": map[string]bool{"readOnlyHint": false, "idempotentHint": true, "destructiveHint": false},
		}
	}
	return []map[string]interface{}{
		tool("holler_thread_follow", "Follow a visible thread as your bound actor. Read channel.thread.get for its revision first. Does not grant access or change anyone else's attention.", map[string]interface{}{"thread_id": map[string]interface{}{"type": "string"}}, "thread_id"),
		tool("holler_thread_unfollow", "Unfollow a thread as your bound actor. Refused while an open request or confirmed work obligates you; new participation may follow it again. Read channel.thread.get first.", map[string]interface{}{"thread_id": map[string]interface{}{"type": "string"}}, "thread_id"),
		tool("holler_channel_set_mode", "Set your own channel attention to channel or threaded, optionally until an absolute timestamp. Read channel.attention.get first. Owner locks and obligations remain enforced; cannot select another actor or lock.", map[string]interface{}{"mode": map[string]interface{}{"type": "string", "enum": []string{"channel", "threaded"}}, "until": map[string]interface{}{"anyOf": []interface{}{map[string]interface{}{"type": "string", "format": "date-time"}, map[string]interface{}{"type": "null"}}}}, "mode"),
		tool("holler_channel_digest_mark", "Advance only your own awareness checkpoint after draining a digest. Use its frozen through_seq and the revision from channel.digest_checkpoint. Does not ACK deliveries, mark messages read, or alter notifications.", map[string]interface{}{"through_seq": map[string]interface{}{"type": "integer", "minimum": 0}}, "through_seq"),
	}
}
