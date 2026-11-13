package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The exact bytes every plugin receives when the daemon cannot be reached.
const degradedHookJSON = `{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "Holler connector state is DEGRADED because startup registration failed. Do not assume peer delivery or ask the user to relay agent messages. Continue independent work where safe and tell the operator to run holler connector doctor."
  }
}
`

func TestMissingDaemonDoesNotBlockLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HOLLER_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))
	t.Setenv("HOLLER_ACTOR", "test")
	t.Setenv("HOLLER_RUN", "run")
	t.Setenv("HOLLER_NAME_MODE", "exact")
	for _, command := range []string{"hook", "session-end"} {
		var out, err bytes.Buffer
		code := run(context.Background(), []string{command, "--harness", "claude"}, strings.NewReader(`{"session_id":"session"}`), &out, &err)
		if code != 0 {
			t.Fatalf("%s blocked: %d %s", command, code, err.String())
		}
		if command == "hook" && out.String() != degradedHookJSON {
			t.Fatalf("DEGRADED hook output changed:\n%s", out.String())
		}
	}
	var out, err bytes.Buffer
	if run(context.Background(), []string{"mcp"}, strings.NewReader(""), &out, &err) == 0 {
		t.Fatal("MCP pretended to connect")
	}
}
