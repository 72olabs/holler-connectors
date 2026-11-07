package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/72olabs/holler-connectors/internal/connector"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// Receipt v1 contains identity and local history metadata, never transcript contents.
// The private core validates it against its managed launch before storing evidence.
func recordAgentSession(ctx context.Context, binding connector.RuntimeBinding, actor, harness, sessionID string, payload []byte, stderr io.Writer) {
	binary := os.Getenv("HOLLER_SESSION_RECORDER")
	if binary == "" || binding.LaunchTag == "" {
		return
	}
	failed := func() {
		fmt.Fprintln(stderr, "holler managed session receipt unavailable; automatic resume remains disabled")
	}
	if !filepath.IsAbs(binary) {
		failed()
		return
	}
	generation, err := strconv.ParseUint(os.Getenv("HOLLER_RUNTIME_GENERATION"), 10, 64)
	if err != nil || generation == 0 {
		failed()
		return
	}
	var hook struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if json.Unmarshal(payload, &hook) != nil || hook.TranscriptPath == "" {
		return
	}
	project, err := os.Getwd()
	if err != nil {
		failed()
		return
	}
	home := ""
	if harness == "codex" {
		home = os.Getenv("CODEX_HOME")
	}
	receipt := struct {
		LaunchTag, Actor, RunID, Harness, SessionID, WorkingDir, TranscriptPath, CodexHome string
		Generation                                                                         uint64
	}{binding.LaunchTag, actor, binding.RunID, harness, sessionID, project, hook.TranscriptPath, home, generation}
	raw, err := json.Marshal(struct {
		Version int         `json:"version"`
		Receipt interface{} `json:"receipt"`
	}{1, receipt})
	if err != nil || len(raw) > 65536 {
		failed()
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "session-receipt")
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if cmd.Run() != nil {
		failed()
	}
}
