package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAttentionToolsHaveFixedSelfOnlyWriteLanes(t *testing.T) {
	for _, tc := range []struct{ name, cap, raw string }{
		{"holler_thread_follow", "channel.thread.follow", `{"channel_id":"c","thread_id":"t","expected_revision":0,"idempotency_key":"follow"}`},
		{"holler_thread_unfollow", "channel.thread.follow", `{"channel_id":"c","thread_id":"t","expected_revision":2,"idempotency_key":"unfollow"}`},
		{"holler_channel_set_mode", "channel.attention.set", `{"channel_id":"c","mode":"threaded","until":"2026-10-26T01:00:00Z","expected_revision":1,"idempotency_key":"mode"}`},
		{"holler_channel_digest_mark", "channel.digest_mark", `{"channel_id":"c","through_seq":42,"expected_revision":0,"idempotency_key":"mark"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &consumeFake{}
			s := &Server{store: f}
			if _, err := s.changeAttention(context.Background(), tc.name, json.RawMessage(tc.raw)); err != nil {
				t.Fatal(err)
			}
			if f.name != tc.cap || f.lane != "write" {
				t.Fatalf("route %s %s", f.name, f.lane)
			}
			var args map[string]interface{}
			if err := json.Unmarshal(f.args, &args); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(tc.name, "holler_thread_") && args["follow"] != (tc.name == "holler_thread_follow") {
				t.Fatal("caller can select follow action", args)
			}
			for _, field := range []string{"actor", "run_id", "gateway", "admin", "target_actor", "locked", "capability", "follow"} {
				f.name = ""
				raw := tc.raw[:len(tc.raw)-1] + `,"` + field + `":"forged"}`
				if _, err := s.changeAttention(context.Background(), tc.name, json.RawMessage(raw)); err == nil || f.name != "" {
					t.Fatalf("accepted selector %s", field)
				}
			}
		})
	}
}

func TestAttentionToolsRejectMissingCASAndUnsupportedModes(t *testing.T) {
	f := &consumeFake{}
	s := &Server{store: f}
	for _, tc := range []struct{ name, raw string }{
		{"holler_thread_follow", `{"channel_id":"c","thread_id":"t","idempotency_key":"k"}`},
		{"holler_thread_unfollow", `{"channel_id":"c","expected_revision":0,"idempotency_key":"k"}`},
		{"holler_channel_set_mode", `{"channel_id":"c","mode":"delegated","expected_revision":0,"idempotency_key":"k"}`},
		{"holler_channel_set_mode", `{"channel_id":"c","mode":"channel","until":"until PR 58 merges","expected_revision":0,"idempotency_key":"k"}`},
		{"holler_channel_set_mode", `{"channel_id":"c","mode":"channel","expected_revision":-1,"idempotency_key":"k"}`},
		{"holler_channel_digest_mark", `{"channel_id":"c","through_seq":-1,"expected_revision":0,"idempotency_key":"k"}`},
		{"holler_channel_digest_mark", `{"channel_id":"c","expected_revision":0,"idempotency_key":"k"}`},
		{"holler_channel_digest_mark", `{"channel_id":"c","through_seq":42,"expected_revision":0}`},
	} {
		if _, err := s.changeAttention(context.Background(), tc.name, json.RawMessage(tc.raw)); err == nil || f.name != "" {
			t.Fatalf("invalid args reached bridge: %s %s", tc.name, tc.raw)
		}
	}
}
