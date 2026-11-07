// Package notify defines the versioned reference-only subprocess protocol used
// by a private Holler daemon to invoke an independently built wake adapter.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/72olabs/holler-connectors/bus"
	"github.com/72olabs/holler-connectors/codexqueue"
	"github.com/72olabs/holler-connectors/opencodeprompt"
	"io"
	"time"
)

const Version = 1
const MaxRequestBytes = 64 << 10

type Request struct {
	Version           int    `json:"version"`
	Harness           string `json:"harness"`
	SessionID         string `json:"session_id"`
	DeliveryHandle    string `json:"delivery_handle"`
	MessageID         string `json:"message_id"`
	SchemaVersion     int    `json:"schema_version"`
	WakeID            string `json:"wake_id,omitempty"`
	DigestThreadCount int64  `json:"digest_thread_count,omitempty"`
	CodexBinary       string `json:"codex_binary,omitempty"`
}
type Result struct {
	Version  int    `json:"version"`
	Accepted bool   `json:"accepted"`
	Detail   string `json:"detail"`
}

func Run(ctx context.Context, input io.Reader) (Result, error) {
	var req Request
	raw, err := io.ReadAll(io.LimitReader(input, MaxRequestBytes+1))
	if err != nil || len(raw) > MaxRequestBytes {
		return Result{}, fmt.Errorf("notification request too large or unreadable")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return Result{}, fmt.Errorf("invalid notification request")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Result{}, fmt.Errorf("expected one notification request")
	}
	if req.Version != Version {
		return Result{}, fmt.Errorf("unsupported notification protocol version")
	}
	if len(req.DeliveryHandle) > 32<<10 {
		return Result{}, fmt.Errorf("delivery handle too large")
	}
	for field, value := range map[string]string{"session_id": req.SessionID, "message_id": req.MessageID} {
		if err := bus.ValidateTextIdentifier(field, value, 256); err != nil {
			return Result{}, err
		}
	}
	if req.WakeID != "" {
		if err := bus.ValidateTextIdentifier("wake_id", req.WakeID, 256); err != nil {
			return Result{}, err
		}
	}
	if req.DigestThreadCount < 0 {
		return Result{}, fmt.Errorf("invalid digest count")
	}
	r := bus.Registration{Harness: req.Harness, SessionID: req.SessionID, DeliveryHandle: req.DeliveryHandle}
	m := bus.Message{ID: req.MessageID, SchemaVersion: req.SchemaVersion, WakeID: req.WakeID, DigestThreadCount: req.DigestThreadCount}
	var detail string
	var accepted bool
	switch req.Harness {
	case "codex":
		detail, accepted = codexqueue.New(req.CodexBinary, 5*time.Second, nil).Notify(ctx, r, m)
	case "opencode":
		detail, accepted = opencodeprompt.New(5*time.Second, nil).Notify(ctx, r, m)
	default:
		return Result{}, fmt.Errorf("unsupported notification harness")
	}
	return Result{Version: Version, Accepted: accepted, Detail: detail}, nil
}
