package notify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectsBodyAndUnknownVersion(t *testing.T) {
	for _, input := range []string{`{"version":2}`, `{"version":1,"body":"private"}`, `{} {}`, `{"version":1,"harness":"other","session_id":"s","message_id":"m"}`} {
		if _, err := Run(context.Background(), strings.NewReader(input)); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}
func TestCodexWakeUsesAdapterWithoutMessageBody(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "codex")
	output := filepath.Join(dir, "args")
	t.Setenv("WAKE_TEST_OUTPUT", output)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$WAKE_TEST_OUTPUT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(Request{Version: 1, Harness: "codex", SessionID: "thread", DeliveryHandle: "thread", MessageID: "message", CodexBinary: binary})
	result, err := Run(context.Background(), strings.NewReader(string(raw)))
	if err != nil || !result.Accepted {
		t.Fatalf("%+v %v", result, err)
	}
	args, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(args), "queue\n--thread\nthread\n--message\n") || !strings.Contains(string(args), "message") {
		t.Fatal(string(args))
	}
}
