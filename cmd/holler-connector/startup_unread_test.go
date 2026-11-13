package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/72olabs/holler-connectors/api"
)

// startupDaemon is the wire state a SessionStart hook sees. A nil catalog
// stands for an older daemon that has no list_capabilities operation at all.
type startupDaemon struct {
	legacy       []map[string]any
	managed      []map[string]any
	catalog      []map[string]any
	managedFails bool
}

type startupCalls struct {
	mu         sync.Mutex
	reads      []string
	consuming  []string
	hydrations []float64
}

func runStartupHook(t *testing.T, daemon startupDaemon) (string, *startupCalls) {
	t.Helper()
	calls := &startupCalls{}
	sessionFixture(t, func(req api.Request) (any, *api.RPCError, bool) {
		calls.mu.Lock()
		defer calls.mu.Unlock()
		var args map[string]any
		_ = json.Unmarshal(req.Args, &args)
		switch req.Op {
		case "register_session":
			return map[string]any{"project_id": "default"}, nil, false
		case "check_inbox":
			return daemon.legacy, nil, false
		case "record_hydration":
			calls.hydrations = append(calls.hydrations, args["unread"].(float64))
			return map[string]any{}, nil, false
		case "list_capabilities":
			if daemon.catalog == nil {
				return nil, &api.RPCError{Code: "bad_request", Message: "unknown operation"}, false
			}
			return daemon.catalog, nil, false
		case "invoke_read_capability":
			name, _ := args["name"].(string)
			calls.reads = append(calls.reads, name)
			switch {
			case name == "channel.inbox" && daemon.managedFails:
				return nil, &api.RPCError{Code: "unavailable", Message: "managed conversations disabled"}, false
			case name == "channel.inbox":
				return daemon.managed, nil, false
			case name == "channel.list":
				return []any{}, nil, false
			}
		case "list_conditions":
			return []any{}, nil, false
		case "claim", "ack", "nack", "extend", "invoke_write_capability":
			calls.consuming = append(calls.consuming, req.Op)
		}
		return nil, &api.RPCError{Code: "bad_request", Message: "fixture unsupported operation"}, false
	})
	var out, stderr bytes.Buffer
	if code := run(context.Background(), []string{"hook", "--harness", "claude"}, strings.NewReader(`{"session_id":"s"}`), &out, &stderr); code != 0 {
		t.Fatalf("hook exit %d: %s", code, stderr.String())
	}
	var hook struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &hook); err != nil {
		t.Fatalf("hook stdout is not JSON: %v: %s", err, out.String())
	}
	return hook.HookSpecificOutput.AdditionalContext, calls
}

var (
	currentCatalog = []map[string]any{
		{"name": "channel.list", "mode": "read", "description": "List channels.", "input_schema": map[string]any{}},
		{"name": "channel.inbox", "mode": "read", "description": "Inspect managed deliveries.", "input_schema": map[string]any{}},
	}
	catalogWithoutInbox = []map[string]any{
		{"name": "channel.list", "mode": "read", "description": "List channels.", "input_schema": map[string]any{}},
	}
	legacyItem = map[string]any{"message_id": "legacy-1", "available": true, "from_actor": "DO-NOT-INJECT-SENDER"}
	// A managed delivery as channel.inbox returns it, with peer-controlled
	// fields that must never reach start-up text.
	managedItem = map[string]any{"state": "queued", "attempt": 0, "message": map[string]any{
		"message_id": "managed-1", "body": "DO-NOT-INJECT-BODY", "sender": "DO-NOT-INJECT-SENDER", "title": "DO-NOT-INJECT-TITLE",
	}}
)

var bigManagedItem = map[string]any{"state": "queued", "attempt": 0, "message": map[string]any{
	"message_id": "managed-big", "body": strings.Repeat("x", 900*1024),
}}

func TestStartupCountsLegacyAndManagedUnread(t *testing.T) {
	for _, tc := range []struct {
		name       string
		daemon     startupDaemon
		want       []string
		reject     []string
		hydrations float64
	}{
		{
			name:   "neither",
			daemon: startupDaemon{legacy: []map[string]any{}, managed: []map[string]any{}, catalog: currentCatalog},
			want:   []string{"No unread messages were present at session start."},
			reject: []string{"could not be checked", "bus_inbox", "holler_channel_inbox"},
		},
		{
			name:       "legacy only",
			daemon:     startupDaemon{legacy: []map[string]any{legacyItem}, managed: []map[string]any{}, catalog: currentCatalog},
			want:       []string{"You have 1 unread message(s)", "1 in the legacy inbox (1 claimable)", "bus_inbox", "bus_ack"},
			reject:     []string{"No unread", "holler_channel_inbox", "could not be checked"},
			hydrations: 1,
		},
		{
			name:       "managed only",
			daemon:     startupDaemon{legacy: []map[string]any{}, managed: []map[string]any{managedItem}, catalog: currentCatalog},
			want:       []string{"You have at least 1 unread message(s)", "at least 1 in conversations (1 claimable)", "holler_channel_inbox", "holler_channel_claim", "holler_channel_ack"},
			reject:     []string{"No unread", "legacy inbox", "bus_inbox", "could not be checked"},
			hydrations: 1,
		},
		{
			name:       "both",
			daemon:     startupDaemon{legacy: []map[string]any{legacyItem}, managed: []map[string]any{managedItem}, catalog: currentCatalog},
			want:       []string{"You have at least 2 unread message(s)", " 1 in the legacy inbox", "at least 1 in conversations", "bus_inbox", "holler_channel_inbox"},
			reject:     []string{"No unread", "could not be checked"},
			hydrations: 2,
		},
		{
			// Two queued messages with near-maximum bodies: the daemon's size
			// budget returns one, with no marker that it stopped early.
			name:       "managed listing cut by body size",
			daemon:     startupDaemon{legacy: []map[string]any{}, managed: []map[string]any{bigManagedItem}, catalog: currentCatalog},
			want:       []string{"You have at least 1 unread message(s)", "at least 1 in conversations"},
			reject:     []string{"No unread", "You have 1 unread"},
			hydrations: 1,
		},
		{
			name:   "older daemon without list_capabilities",
			daemon: startupDaemon{legacy: []map[string]any{}},
			want:   []string{"No unread legacy messages", "Conversation deliveries (direct chats and rooms) could not be checked", "not proof that nothing is waiting"},
			reject: []string{"No unread messages were present"},
		},
		{
			name:   "older daemon without channel.inbox",
			daemon: startupDaemon{legacy: []map[string]any{}, catalog: catalogWithoutInbox},
			want:   []string{"No unread legacy messages", "could not be checked"},
			reject: []string{"No unread messages were present"},
		},
		{
			name:       "managed inbox fails with legacy mail",
			daemon:     startupDaemon{legacy: []map[string]any{legacyItem}, catalog: currentCatalog, managedFails: true},
			want:       []string{"You have 1 unread message(s)", "bus_inbox", "could not be checked"},
			reject:     []string{"No unread", "holler_channel_inbox"},
			hydrations: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, calls := runStartupHook(t, tc.daemon)
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in:\n%s", want, text)
				}
			}
			for _, reject := range append(tc.reject, "DO-NOT-INJECT", "managed-1", "managed-big", "legacy-1", "xxxx") {
				if strings.Contains(text, reject) {
					t.Errorf("unexpected %q in:\n%s", reject, text)
				}
			}
			calls.mu.Lock()
			defer calls.mu.Unlock()
			if len(calls.consuming) != 0 {
				t.Fatalf("SessionStart consumed deliveries: %v", calls.consuming)
			}
			for _, read := range calls.reads {
				if read != "channel.inbox" && read != "channel.list" {
					t.Fatalf("unexpected capability read %q", read)
				}
			}
			if len(calls.hydrations) != 1 || calls.hydrations[0] != tc.hydrations {
				t.Fatalf("hydration unread=%v, want %v", calls.hydrations, tc.hydrations)
			}
		})
	}
}
