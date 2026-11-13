package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/72olabs/holler-connectors/api"
)

type memberPostFake struct {
	consumeFake
	supported bool
}

func (f *memberPostFake) Supports(_ context.Context, capability string) (bool, error) {
	return f.supported && capability == api.ChannelMemberPostCapability, nil
}

func TestChannelPostToolUsesOnlyTheMemberPostLane(t *testing.T) {
	if memberPostCapability != api.ChannelMemberPostCapability {
		t.Fatal("the MCP gate and the daemon's hello capability differ")
	}
	raw := `{"channel_id":"chn_1","thread_id":"thr_1","in_reply_to":"msg_1","expected_policy_revision":4,"idempotency_key":"k","body":{"text":"done"},"attention_targets":["b"],"respondent":"human:owner"}`
	f := &memberPostFake{supported: true}
	s := &Server{store: f}
	if _, err := s.memberPost(context.Background(), json.RawMessage(raw)); err != nil {
		t.Fatal(err)
	}
	if f.lane != "write" || f.name != "channel.member_post" {
		t.Fatalf("routed to %s %s", f.lane, f.name)
	}
	var got, want map[string]interface{}
	json.Unmarshal(f.args, &got)
	json.Unmarshal([]byte(raw), &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments changed: %s", f.args)
	}
	// Anything the lane doesn't carry is refused here, before the daemon.
	for _, field := range []string{"references", "response_to", "work_proposal", "confirm_work_proposal", "authorization_source_id", "actor", "capability"} {
		f.name = ""
		extra := raw[:len(raw)-1] + `,"` + field + `":"x"}`
		if _, err := s.memberPost(context.Background(), json.RawMessage(extra)); err == nil || f.name != "" {
			t.Fatalf("accepted %s", field)
		}
	}
}

func TestChannelPostToolNeedsAnEngineThatOffersIt(t *testing.T) {
	raw := json.RawMessage(`{"channel_id":"chn_1","expected_policy_revision":1,"idempotency_key":"k","body":{"text":"x"}}`)
	for name, store := range map[string]*Server{
		"older engine":            {store: &memberPostFake{supported: false}},
		"no capability handshake": {store: &consumeFake{}},
	} {
		_, err := store.memberPost(context.Background(), raw)
		if !errors.Is(err, errMemberPostUnavailable) {
			t.Fatalf("%s: %v", name, err)
		}
		if f, ok := store.store.(*memberPostFake); ok && f.name != "" {
			t.Fatalf("%s reached the daemon", name)
		}
	}
}
