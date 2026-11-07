package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/72olabs/holler-connectors/bus"
)

// memberPostCapability is api.ChannelMemberPostCapability (a test keeps them
// equal): the daemon's hello says it offers channel.member_post.
const memberPostCapability = "channel-member-post-v1"

var errMemberPostUnavailable = errors.New("member_post_unavailable: this Holler engine can't post without approval; ask the user, then use holler_write channel.post")

// memberPost is the one narrow post. The daemon decides whether it is allowed
// (C5 rules A and B, the round and question caps, the channel's switch); this
// surface only refuses fields the lane never carries and an engine without it.
func (s *Server) memberPost(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	bridge, ok := s.store.(capabilityStore)
	if !ok {
		return nil, bus.ErrChannelCapability
	}
	gate, ok := s.store.(interface {
		Supports(context.Context, string) (bool, error)
	})
	if !ok {
		return nil, errMemberPostUnavailable
	}
	supported, err := gate.Supports(ctx, memberPostCapability)
	if err != nil {
		return nil, err
	}
	if !supported {
		return nil, errMemberPostUnavailable
	}
	var a bus.ChannelMemberPost
	if err := decodeStrict(raw, &a); err != nil {
		return nil, err
	}
	args, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	return bridge.InvokeWriteCapability(ctx, "channel.member_post", args)
}

// roomAccessCapability is api.RoomAccessCapability (a test keeps them equal).
const roomAccessCapability = "channel-room-access-v1"

var errRoomAccessUnavailable = errors.New("room_access_unavailable: this Holler engine has no public rooms or join requests; ask the user to add you to the room")

// roomAccess backs the narrow room tools. Each acts only for the bound actor
// and never widens access beyond policy: join admits to public rooms only, a
// request grants nothing, and leave only removes the caller. Deciding requests
// and adding others stay on holler_write.
func (s *Server) roomAccess(ctx context.Context, name string, raw json.RawMessage) (interface{}, error) {
	bridge, ok := s.store.(capabilityStore)
	if !ok {
		return nil, bus.ErrChannelCapability
	}
	gate, ok := s.store.(interface {
		Supports(context.Context, string) (bool, error)
	})
	if !ok {
		return nil, errRoomAccessUnavailable
	}
	supported, err := gate.Supports(ctx, roomAccessCapability)
	if err != nil {
		return nil, err
	}
	if !supported {
		return nil, errRoomAccessUnavailable
	}
	var args interface{}
	operation := ""
	switch name {
	case "holler_channel_discover":
		var a bus.ChannelDiscover
		args, operation = &a, "channel.discover"
	case "holler_channel_join":
		var a bus.ChannelJoin
		args, operation = &a, "channel.join"
	case "holler_channel_leave":
		var a bus.ChannelLeave
		args, operation = &a, "channel.leave"
	case "holler_channel_request_access":
		var a bus.JoinRequestCreate
		args, operation = &a, "channel.join_request.create"
	default:
		return nil, bus.ErrChannelCapability
	}
	if err := decodeStrict(raw, args); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	if operation == "channel.discover" {
		return bridge.InvokeReadCapability(ctx, operation, encoded)
	}
	return bridge.InvokeWriteCapability(ctx, operation, encoded)
}

// This surface cannot select a capability, sender, recipient or channel. It
// consumes only the bound actor's delivery; other writes use the write bridge.
func (s *Server) consumeManaged(ctx context.Context, name string, raw json.RawMessage) (interface{}, error) {
	bridge, ok := s.store.(capabilityStore)
	if !ok {
		return nil, bus.ErrChannelCapability
	}
	if name == "holler_channel_inbox" {
		var a struct {
			Limit int `json:"limit"`
		}
		if err := decodeStrict(raw, &a); err != nil {
			return nil, err
		}
		args, _ := json.Marshal(a)
		return bridge.InvokeReadCapability(ctx, "channel.inbox", args)
	}
	if name == "holler_channel_claim" {
		var a struct {
			ID string `json:"message_id"`
		}
		if err := decodeStrict(raw, &a); err != nil {
			return nil, err
		}
		if a.ID == "" {
			return nil, bus.ErrInvalid
		}
		args, _ := json.Marshal(a)
		return bridge.InvokeWriteCapability(ctx, "channel.claim", args)
	}
	var id, token, reason string
	var seconds int
	action := ""
	switch name {
	case "holler_channel_ack":
		var a struct {
			ID    string `json:"message_id"`
			Token string `json:"lease_token"`
		}
		if err := decodeStrict(raw, &a); err != nil {
			return nil, err
		}
		id, token, action = a.ID, a.Token, "ack"
	case "holler_channel_extend":
		var a struct {
			ID      string `json:"message_id"`
			Token   string `json:"lease_token"`
			Seconds int    `json:"lease_seconds"`
		}
		if err := decodeStrict(raw, &a); err != nil {
			return nil, err
		}
		id, token, seconds, action = a.ID, a.Token, a.Seconds, "extend"
	case "holler_channel_nack":
		var a struct {
			ID     string `json:"message_id"`
			Token  string `json:"lease_token"`
			Reason string `json:"reason"`
			Final  bool   `json:"final"`
		}
		if err := decodeStrict(raw, &a); err != nil {
			return nil, err
		}
		id, token, reason, action = a.ID, a.Token, a.Reason, "nack"
		if a.Final {
			action = "dead_letter"
		}
	default:
		return nil, bus.ErrChannelCapability
	}
	if id == "" || token == "" {
		return nil, bus.ErrInvalid
	}
	args, _ := json.Marshal(map[string]interface{}{"message_id": id, "lease_token": token, "action": action, "reason": reason, "lease_seconds": seconds})
	return bridge.InvokeWriteCapability(ctx, "channel.delivery", args)
}

func channelTool(name, description string, props map[string]interface{}, required []string, read, idempotent bool) map[string]interface{} {
	return map[string]interface{}{"name": name, "description": description, "inputSchema": map[string]interface{}{"type": "object", "properties": props, "additionalProperties": false, "required": required}, "annotations": map[string]bool{"readOnlyHint": read, "idempotentHint": idempotent, "destructiveHint": false}}
}

func channelToolDefinitions() []map[string]interface{} {
	str := map[string]interface{}{"type": "string"}
	integer := map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 86400}
	return []map[string]interface{}{
		channelTool("holler_channel_inbox", "Inspect your own managed deliveries without claiming or marking read. Requires managed conversations to be enabled.", map[string]interface{}{"limit": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 100}}, []string{}, true, true),
		channelTool("holler_channel_claim", "Claim one managed message addressed to your bound identity for five minutes. Process it, then use holler_channel_ack. Observers cannot claim agent deliveries.", map[string]interface{}{"message_id": str}, []string{"message_id"}, false, false),
		channelTool("holler_channel_ack", "Acknowledge your processed managed delivery using its active lease token. Does not answer a response request or mark human history read.", map[string]interface{}{"message_id": str, "lease_token": str}, []string{"message_id", "lease_token"}, false, true),
		channelTool("holler_channel_extend", "Extend your own active managed delivery lease before it expires.", map[string]interface{}{"message_id": str, "lease_token": str, "lease_seconds": integer}, []string{"message_id", "lease_token", "lease_seconds"}, false, true),
		channelTool("holler_channel_nack", "Release your managed delivery for retry, or mark it dead-lettered with final=true. Reason stays recipient-private.", map[string]interface{}{"message_id": str, "lease_token": str, "reason": str, "final": map[string]interface{}{"type": "boolean"}}, []string{"message_id", "lease_token"}, false, false),
		channelTool("holler_channel_post", "Post in a Holler channel without a separate approval, only where Holler allows it: a channel a person created that you post in, or a reply in the thread of a message that named you, addressed only to its sender. Refresh channel.get first and pass its policy_revision. No references, work proposals or answers to questions. Holler refuses anything else; for that, ask the user and use holler_write channel.post.", map[string]interface{}{"channel_id": str, "thread_id": str, "in_reply_to": str, "expected_policy_revision": map[string]interface{}{"type": "integer", "minimum": 0}, "idempotency_key": str, "body": map[string]interface{}{"type": "object"}, "attention_targets": map[string]interface{}{"type": "array", "items": str}, "respondent": str}, []string{"channel_id", "expected_policy_revision", "idempotency_key", "body"}, false, true),
	}
}

// roomAccessToolDefinitions are the narrow room tools, backed by roomAccess and
// listed after holler_channel_post. Connector manifests pre-approve them.
func roomAccessToolDefinitions() []map[string]interface{} {
	str := map[string]interface{}{"type": "string"}
	return []map[string]interface{}{
		channelTool("holler_channel_discover", "List every named Holler room, public and private, with title, purpose, creator, members, visibility and your latest join request. Metadata only, never messages. Join a public room with holler_channel_join; ask for a private one with holler_channel_request_access.", map[string]interface{}{"cursor": str, "limit": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 200}}, []string{}, true, true),
		channelTool("holler_channel_join", "Join a public Holler room yourself. You see history from the join on. Private rooms refuse; use holler_channel_request_access.", map[string]interface{}{"channel_id": str, "idempotency_key": str}, []string{"channel_id", "idempotency_key"}, false, true),
		channelTool("holler_channel_leave", "Leave a Holler room you are done with. Your pending deliveries and open questions there are revoked, and rejoining restores no history. A room's creator can't leave.", map[string]interface{}{"channel_id": str, "idempotency_key": str}, []string{"channel_id", "idempotency_key"}, false, true),
		channelTool("holler_channel_request_access", "Ask a private Holler room's managers to let you in, with a short reason (at most 500 characters). Grants nothing until a manager approves; approval wakes you in the room, and a decline shows as my_request in holler_channel_discover. One open request per room; requests expire after 7 days.", map[string]interface{}{"channel_id": str, "reason": map[string]interface{}{"type": "string", "maxLength": 500}, "idempotency_key": str}, []string{"channel_id", "reason", "idempotency_key"}, false, true),
	}
}
