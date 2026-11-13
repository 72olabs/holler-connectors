package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/72olabs/holler-connectors/bus"
)

type statusFake struct {
	consumeFake
	legacy  []bus.InboxItem
	catalog []bus.CapabilityDescriptor
	managed string
}

func (f *statusFake) CheckInbox(context.Context, string, int) ([]bus.InboxItem, error) {
	return f.legacy, nil
}
func (f *statusFake) ListConditions(context.Context, bool, int) ([]bus.OperatorCondition, error) {
	return nil, nil
}
func (f *statusFake) ListCapabilities(context.Context) ([]bus.CapabilityDescriptor, error) {
	return f.catalog, nil
}
func (f *statusFake) InvokeReadCapability(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	if name != "channel.inbox" {
		panic(name)
	}
	return json.RawMessage(f.managed), nil
}

func TestBusStatusCountsManagedDeliveries(t *testing.T) {
	inbox := []bus.CapabilityDescriptor{{Name: "channel.inbox", Mode: bus.CapabilityRead}}
	for _, tc := range []struct {
		name string
		fake *statusFake
		want map[string]interface{}
	}{
		{"managed only", &statusFake{catalog: inbox, managed: `[{"state":"queued"}]`},
			map[string]interface{}{"unread": 1, "available": 1, "legacy_unread": 0, "managed_unread": 1, "managed_checked": true, "counts_truncated": true, "managed_unread_complete": false}},
		{"empty", &statusFake{catalog: inbox, managed: `[]`},
			map[string]interface{}{"unread": 0, "managed_checked": true, "counts_truncated": false, "managed_unread_complete": true}},
		{"both", &statusFake{legacy: []bus.InboxItem{{Available: true}}, catalog: inbox, managed: `[{"state":"queued"}]`},
			map[string]interface{}{"unread": 2, "available": 2, "legacy_unread": 1, "managed_unread": 1, "managed_checked": true}},
		{"older daemon", &statusFake{legacy: []bus.InboxItem{{Available: true}}},
			map[string]interface{}{"unread": 1, "legacy_unread": 1, "managed_unread": 0, "managed_checked": false, "counts_truncated": false, "managed_unread_complete": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{store: tc.fake, config: Config{Actor: "a", RunID: "r"}}
			result, err := s.callTool(context.Background(), "bus_status", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			got := result.(map[string]interface{})
			for key, want := range tc.want {
				if got[key] != want {
					t.Errorf("%s = %v, want %v", key, got[key], want)
				}
			}
		})
	}
}
