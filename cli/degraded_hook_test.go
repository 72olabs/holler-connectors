package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/72olabs/holler-connectors/bus"
	"github.com/72olabs/holler-connectors/connector"
)

func TestCLIHookExplainsStaleBindingWithoutBlockingHarnessStartup(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := writeDegradedHook(&stdout, &stderr, fmt.Errorf("hello refused: %w", bus.ErrBindingStale)); err != nil {
		t.Fatal(err)
	}
	var output connector.HookOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode %s: %v", stdout.Bytes(), err)
	}
	if !strings.Contains(stderr.String(), "relaunch") ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "STALE") ||
		!strings.Contains(output.HookSpecificOutput.AdditionalContext, "Relaunch") {
		t.Fatalf("stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}
