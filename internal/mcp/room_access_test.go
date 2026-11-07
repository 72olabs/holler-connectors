package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/72olabs/holler-connectors/api"
	"github.com/72olabs/holler-connectors/bus"
)

type roomAccessFake struct {
	consumeFake
	supported bool
}

func (f *roomAccessFake) Supports(_ context.Context, capability string) (bool, error) {
	return f.supported && capability == api.RoomAccessCapability, nil
}

func TestRoomAccessToolsUseFixedLanesForTheBoundActor(t *testing.T) {
	if roomAccessCapability != api.RoomAccessCapability {
		t.Fatal("the MCP gate and the daemon's hello capability differ")
	}
	for _, tc := range []struct{ tool, raw, lane, op string }{
		{"holler_channel_discover", `{"cursor":"chn_1","limit":20}`, "read", "channel.discover"},
		{"holler_channel_join", `{"channel_id":"chn_1","idempotency_key":"k"}`, "write", "channel.join"},
		{"holler_channel_leave", `{"channel_id":"chn_1","idempotency_key":"k"}`, "write", "channel.leave"},
		{"holler_channel_request_access", `{"channel_id":"chn_1","reason":"I review deploys","idempotency_key":"k"}`, "write", "channel.join_request.create"},
	} {
		f := &roomAccessFake{supported: true}
		s := &Server{store: f}
		if _, err := s.roomAccess(context.Background(), tc.tool, json.RawMessage(tc.raw)); err != nil {
			t.Fatalf("%s: %v", tc.tool, err)
		}
		var got, want map[string]interface{}
		json.Unmarshal(f.args, &got)
		json.Unmarshal([]byte(tc.raw), &want)
		if f.lane != tc.lane || f.name != tc.op || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s routed to %s %s %s", tc.tool, f.lane, f.name, f.args)
		}
		// No tool can act for another actor or reach another operation.
		for _, field := range []string{"actor", "requester", "decision", "capability", "visibility"} {
			f.name = ""
			extra := tc.raw[:len(tc.raw)-1] + `,"` + field + `":"x"}`
			if _, err := s.roomAccess(context.Background(), tc.tool, json.RawMessage(extra)); err == nil || f.name != "" {
				t.Fatalf("%s accepted %s", tc.tool, field)
			}
		}
	}
}

func TestRoomAccessToolsNeedAnEngineThatOffersThem(t *testing.T) {
	for name, s := range map[string]*Server{
		"older engine":            {store: &roomAccessFake{supported: false}},
		"no capability handshake": {store: &consumeFake{}},
	} {
		for _, tool := range []string{"holler_channel_discover", "holler_channel_join", "holler_channel_leave", "holler_channel_request_access"} {
			_, err := s.roomAccess(context.Background(), tool, json.RawMessage(`{"channel_id":"chn_1","idempotency_key":"k"}`))
			if !errors.Is(err, errRoomAccessUnavailable) || !strings.HasPrefix(err.Error(), "room_access_unavailable:") {
				t.Fatalf("%s %s: %v", name, tool, err)
			}
			if f, ok := s.store.(*roomAccessFake); ok && f.name != "" {
				t.Fatalf("%s %s reached the daemon", name, tool)
			}
		}
	}
}

// Each tool's advertised schema is exactly the request it decodes, and the
// tools are listed right after holler_channel_post (HOL-83).
func TestRoomAccessToolSchemasMatchTheirRequests(t *testing.T) {
	requests := map[string]interface{}{
		"holler_channel_discover": bus.ChannelDiscover{}, "holler_channel_join": bus.ChannelJoin{},
		"holler_channel_leave": bus.ChannelLeave{}, "holler_channel_request_access": bus.JoinRequestCreate{},
	}
	definitions := roomAccessToolDefinitions()
	if len(definitions) != len(requests) {
		t.Fatalf("%d definitions", len(definitions))
	}
	names := ToolNames()
	post := slices.Index(names, "holler_channel_post")
	for i, d := range definitions {
		name := d["name"].(string)
		if post < 0 || slices.Index(names, name) != post+1+i {
			t.Fatalf("%s is not listed after holler_channel_post: %v", name, names)
		}
		schema := d["inputSchema"].(map[string]interface{})
		var props []string
		for p := range schema["properties"].(map[string]interface{}) {
			props = append(props, p)
		}
		sort.Strings(props)
		var fields []string
		rt := reflect.TypeOf(requests[name])
		for i := 0; i < rt.NumField(); i++ {
			fields = append(fields, strings.Split(rt.Field(i).Tag.Get("json"), ",")[0])
		}
		sort.Strings(fields)
		if !slices.Equal(props, fields) {
			t.Fatalf("%s schema %v, request %v", name, props, fields)
		}
	}
}
