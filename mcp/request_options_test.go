package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The words an agent reads, at the moment it asks a person something, about
// how to ask: pinned, as the start-up text is. Real Claude Code and Codex
// agents wrote a Request's choices into its sentence 5 times in 5 until a
// person told them to use the options field, after which 5 of 5 did; these
// words tell every agent that, where it reads them.
const wantRequestShape = "To ask a person to decide, set respondent to their human: id and give body a one-line question. When the answer is one of a few you can name, put each in body.options as a short label (a yes/no is two); only if you have a reason to prefer one, put it first with recommended 0. The person then answers with one key. When the answers cannot be listed, ask in free text and do not make options up."

const wantBodyDescription = `The post: {"text":"…"}. A request to a person (with respondent): {"question":"Which branch should the fix go on?","options":["main","release-2.1"]}.`

// The refusal of a request to a person whose body says nothing about how it
// is to be answered: the whole lesson, for an agent that read nothing else.
const wantUnshapedRefusal = `Not posted: this asks a person (respondent) without saying how they can answer. Resend with the same idempotency_key and body {"question":"…","options":["…","…"]} when the answers can be listed (a yes/no is two options), or {"question":"…"} alone when they cannot; do not make options up. Only if you have a reason to prefer one option, add "recommended": its index and "why": one sentence; keep labels plain, never "(recommended)". If no answer is needed, resend without respondent.`

// postResultFake answers holler_channel_post's lane with a post result.
type postResultFake struct{ memberPostFake }

func (f *postResultFake) InvokeWriteCapability(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	if _, err := f.memberPostFake.InvokeWriteCapability(ctx, name, args); err != nil {
		return nil, err
	}
	return json.RawMessage(`{"message":{"message_id":"msg_1"},"duplicate":false,"recipients":["human:owner"]}`), nil
}

func (f *postResultFake) HeartbeatRegistrations(context.Context, string, string, time.Duration) (int, error) {
	return 1, nil
}

func postToolDefinition(t *testing.T) map[string]interface{} {
	t.Helper()
	for _, tool := range channelToolDefinitions() {
		if tool["name"] == "holler_channel_post" {
			return tool
		}
	}
	t.Fatal("no holler_channel_post")
	return nil
}

func TestPostToolTellsHowToAskAPerson(t *testing.T) {
	tool := postToolDefinition(t)
	description := tool["description"].(string)
	if !strings.HasSuffix(description, " "+wantRequestShape) {
		t.Fatalf("holler_channel_post does not end with the request shape:\n%s", description)
	}
	body := tool["inputSchema"].(map[string]interface{})["properties"].(map[string]interface{})["body"].(map[string]interface{})
	if body["type"] != "object" || body["description"] != wantBodyDescription {
		t.Fatalf("body's schema: %v", body)
	}
}

// A request to a person whose body says nothing about how it is to be answered
// (a plain string, or an object with neither question nor options, unless it
// is an approval or a blocked report) is refused before anything reaches the
// engine: real agents now call holler_channel_post without reading its
// description, and an error is the one result every harness shows the model.
// Everything else passes unchanged. Only the structure is read, never the words.
func TestPostToolRefusesARequestToAPersonWithoutItsShape(t *testing.T) {
	for _, tc := range []struct {
		name, respondent, body string
		refused                bool
	}{
		{"a string body", "human:owner", `"Which title: Launch notes or Release notes?"`, true},
		{"text", "human:owner", `{"text":"Launch notes or Release notes?"}`, true},
		{"text with empty options", "human:owner", `{"text":"Which title?","options":[]}`, true},
		{"neither question nor options", "human:owner", `{"summary":"Which title?"}`, true},
		{"a human: id in another case", "Human:owner", `{"text":"Which title?"}`, true},
		{"a question alone (its answers cannot be listed)", "human:owner", `{"question":"What should the release notes say?"}`, false},
		{"a question with options", "human:owner", `{"question":"Which branch?","options":["main","release-2.1"]}`, false},
		{"text with options", "human:owner", `{"text":"Which branch?","options":["main","release-2.1"]}`, false},
		{"object options and a recommendation", "human:owner", `{"question":"Ship?","options":[{"label":"Wait"},{"label":"Ship"}],"recommended":0,"why":"CI is red."}`, false},
		{"an approval", "human:owner", `{"kind":"approval","action":"Merge PR #47"}`, false},
		{"a blocked report", "human:owner", `{"kind":"blocked","problem":"CI times out"}`, false},
		{"an agent respondent", "claude-82a8af", `{"text":"Review PR #47 now or after lunch?"}`, false},
		{"an agent respondent, a string body", "claude-82a8af", `"Review PR #47 now?"`, false},
		{"no respondent", "", `{"text":"Launch notes or Release notes?"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &postResultFake{memberPostFake{supported: true}}
			s, err := New(f, Config{Actor: "test", RunID: "run"})
			if err != nil {
				t.Fatal(err)
			}
			args := map[string]interface{}{"channel_id": "chn_1", "expected_policy_revision": 1, "idempotency_key": "k", "body": json.RawMessage(tc.body)}
			if tc.respondent != "" {
				args["respondent"] = tc.respondent
			}
			params, _ := json.Marshal(map[string]interface{}{"name": "holler_channel_post", "arguments": args})
			var output bytes.Buffer
			if err := s.Run(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(params)+"}\n"), &output); err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
				Result *struct {
					Content []struct{ Type, Text string } `json:"content"`
				} `json:"result"`
			}
			if err := json.Unmarshal(output.Bytes(), &wire); err != nil {
				t.Fatalf("%s (%v)", output.String(), err)
			}
			if tc.refused {
				if wire.Error == nil || wire.Error.Message != wantUnshapedRefusal {
					t.Fatalf("want the refusal, got %s", output.String())
				}
				if f.name != "" {
					t.Fatalf("a refused request reached the engine (%s %s)", f.lane, f.name)
				}
				return
			}
			if wire.Error != nil || wire.Result == nil || len(wire.Result.Content) != 1 || f.name != "channel.member_post" {
				t.Fatalf("want it posted unchanged, got %s (engine saw %q)", output.String(), f.name)
			}
			var sent map[string]json.RawMessage
			if err := json.Unmarshal(f.args, &sent); err != nil || string(sent["body"]) != tc.body {
				t.Fatalf("the body changed on its way: %s", f.args)
			}
		})
	}
}
