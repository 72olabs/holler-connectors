package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/72olabs/holler-connectors/connector"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedReceiptHandoff(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "core")
	output := filepath.Join(dir, "receipt")
	t.Setenv("RECEIPT_OUTPUT", output)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n[ \"$1\" = session-receipt ] || exit 2\ncat > \"$RECEIPT_OUTPUT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOLLER_SESSION_RECORDER", binary)
	t.Setenv("HOLLER_RUNTIME_GENERATION", "7")
	var stderr bytes.Buffer
	recordAgentSession(context.Background(), connector.RuntimeBinding{LaunchTag: "launch", RunID: "run"}, "actor", "claude", "session", []byte(`{"transcript_path":"/private/history","prompt":"do not forward"}`), &stderr)
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Version int
		Receipt struct {
			Actor, LaunchTag, RunID, TranscriptPath string
			Generation                              uint64
		}
	}
	if json.Unmarshal(raw, &request) != nil || request.Version != 1 || request.Receipt.Actor != "actor" || request.Receipt.Generation != 7 || request.Receipt.RunID != "run" {
		t.Fatalf("invalid receipt: %s", raw)
	}
	if strings.Contains(string(raw), "do not forward") || stderr.Len() != 0 {
		t.Fatal("unexpected output")
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho secret >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	recordAgentSession(context.Background(), connector.RuntimeBinding{LaunchTag: "launch", RunID: "run"}, "actor", "claude", "session", []byte(`{"transcript_path":"/private/history"}`), &stderr)
	if !strings.Contains(stderr.String(), "unavailable") || strings.Contains(stderr.String(), "secret") {
		t.Fatal("unsafe error")
	}
}
