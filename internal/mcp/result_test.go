package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type resultStore struct {
	consumeFake
	result json.RawMessage
}

type countedResult struct{ calls int }

func (v *countedResult) MarshalJSON() ([]byte, error) {
	v.calls++
	return []byte(`{"count":9007199254740993}`), nil
}

func TestToolResultGoValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value interface{}
		want  string
	}{
		{"nil", nil, `{"result":null}`},
		{"nil map", map[string]string(nil), `{"result":null}`},
		{"nil slice", []string(nil), `{"result":null}`},
		{"empty slice", []string{}, `{"result":[]}`},
		{"array", [1]string{"message"}, `{"result":["message"]}`},
		{"struct", struct {
			Count int `json:"count"`
		}{1}, `{"count":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := toolResult(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(result["structuredContent"])
			if err != nil {
				t.Fatal(err)
			}
			assertJSONBytes(t, encoded, tc.want)
		})
	}
	v := &countedResult{}
	result, err := toolResult(v)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(result); err != nil {
		t.Fatal(err)
	}
	if v.calls != 1 {
		t.Fatalf("marshaled %d times", v.calls)
	}
	if _, err := toolResult(make(chan int)); err == nil {
		t.Fatal("unsupported value accepted")
	}
}

func (f *resultStore) HeartbeatRegistrations(context.Context, string, string, time.Duration) (int, error) {
	return 0, nil
}

func (f *resultStore) InvokeReadCapability(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return f.result, f.err
}

func (f *resultStore) InvokeWriteCapability(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return f.result, f.err
}

// Exercise tools/call and the final JSON encoder, not just the tool dispatcher.
func TestToolResultWireEnvelope(t *testing.T) {
	for _, tool := range []string{
		`{"name":"holler_channel_inbox","arguments":{"limit":20}}`,
		`{"name":"holler_read","arguments":{"capability":"channel.list","arguments":{}}}`,
		`{"name":"holler_read","arguments":{"capability":"channel.responses","arguments":{}}}`,
		`{"name":"holler_write","arguments":{"capability":"channel.post","arguments":{}}}`,
		`{"name":"holler_channel_claim","arguments":{"message_id":"m"}}`,
	} {
		for _, value := range []string{`[]`, `[{"message_id":"m"}]`, `null`, `true`, `"hello"`, `9007199254740993`, `{"count":9007199254740993}`, `{}`, `{"result":"already an object"}`} {
			t.Run(tool+"/"+value, func(t *testing.T) {
				wire := runResultTool(t, tool, &resultStore{result: json.RawMessage(value)})
				if _, ok := wire["error"]; ok {
					t.Fatalf("unexpected error: %s", wire["error"])
				}
				var result struct {
					Content           []struct{ Type, Text string }
					StructuredContent map[string]json.RawMessage
				}
				if err := json.Unmarshal(wire["result"], &result); err != nil {
					t.Fatalf("result must contain object structuredContent: %v; wire=%s", err, wire["result"])
				}
				if result.StructuredContent == nil || len(result.Content) != 1 || result.Content[0].Type != "text" {
					t.Fatalf("invalid result: %s", wire["result"])
				}
				assertJSONBytes(t, []byte(result.Content[0].Text), value)
				if strings.HasPrefix(value, "{") {
					encoded, err := json.Marshal(result.StructuredContent)
					if err != nil {
						t.Fatal(err)
					}
					assertJSONBytes(t, encoded, value)
				} else {
					if len(result.StructuredContent) != 1 {
						t.Fatal(result.StructuredContent)
					}
					assertJSONBytes(t, result.StructuredContent["result"], value)
				}
			})
		}
	}
}

func TestToolResultWireErrors(t *testing.T) {
	for _, f := range []*resultStore{
		{result: json.RawMessage(`{broken`)},
		{consumeFake: consumeFake{err: errors.New("capability unavailable")}},
	} {
		wire := runResultTool(t, `{"name":"holler_channel_inbox","arguments":{}}`, f)
		if _, ok := wire["error"]; !ok {
			t.Fatalf("missing RPC error: %v", wire)
		}
		if _, ok := wire["result"]; ok {
			t.Fatalf("error returned success result: %v", wire)
		}
	}
}

func runResultTool(t *testing.T, params string, f *resultStore) map[string]json.RawMessage {
	t.Helper()
	s, err := New(f, Config{Actor: "test", RunID: "run"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` + params + "}\n"
	if err := s.Run(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	return wire
}

func assertJSONBytes(t *testing.T, got []byte, want string) {
	t.Helper()
	var compact bytes.Buffer
	if err := json.Compact(&compact, got); err != nil {
		t.Fatal(err)
	}
	if compact.String() != want {
		t.Fatalf("got %s; want %s", compact.String(), want)
	}
}
