package connector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/72olabs/holler-connectors/bus"
	"github.com/72olabs/holler-connectors/codexqueue"
	"github.com/72olabs/holler-connectors/internal/untrusted"
	"github.com/72olabs/holler-connectors/opencodeprompt"
)

type Store interface {
	ClaimAliasIfAbsent(context.Context, bus.AliasClaimRequest) (bus.AliasClaimResult, error)
	RegisterSession(context.Context, bus.RegistrationRequest) (bus.Registration, error)
	LiveRegistrations(context.Context, string) ([]bus.Registration, error)
	CheckInbox(context.Context, string, int) ([]bus.InboxItem, error)
	RecordHydration(context.Context, string, string, string, string, string, int) error
	ExpireRegistration(context.Context, string, string, string, string) error
}

// Runtime owns client-side lifecycle hydration only. Authorization, notification
// scheduling and registration selection remain in the daemon.
type Runtime struct {
	store      Store
	sessionTTL time.Duration
	getenv     func(string) string
}
type CommandRunner func(context.Context, string, ...string) (stdout, stderr string, exitCode int, err error)
type Option func(*Runtime)

func WithSessionTTL(ttl time.Duration) Option { return func(r *Runtime) { r.sessionTTL = ttl } }
func WithEnvironment(getenv func(string) string) Option {
	return func(r *Runtime) { r.getenv = getenv }
}
func New(store Store, options ...Option) *Runtime {
	r := &Runtime{store: store, sessionTTL: 5 * time.Minute, getenv: os.Getenv}
	for _, option := range options {
		option(r)
	}
	return r
}

type SessionConfig struct {
	Actor          string
	RunID          string
	Harness        string
	ProjectID      string
	AttentionMode  string
	DeliveryHandle string
	WorkingDir     string
	NameMode       bus.NameMode
}

type HookOutput struct {
	HookSpecificOutput HookSpecificOutput `json:"hookSpecificOutput"`
}

type HookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

func (r *Runtime) SessionStart(ctx context.Context, config SessionConfig, input io.Reader) (HookOutput, error) {
	payload, err := ParseLifecycleInput(input)
	if err != nil {
		return HookOutput{}, err
	}
	sessionID := payload.SessionID
	deliveryHandle := config.DeliveryHandle
	attentionMode := config.AttentionMode
	wakeContext := ""
	if config.Harness == "codex" && config.AttentionMode != AttentionStartupOnly && deliveryHandle == "" {
		deliveryHandle, err = codexqueue.HandleFromEnvironment(sessionID)
		if errors.Is(err, codexqueue.ErrInvalidHome) {
			attentionMode = AttentionStartupOnly
			deliveryHandle = codexqueue.UnavailableHomeHandle(sessionID)
			wakeContext = " Live wake is unavailable (codex_home_invalid); durable messaging remains available with startup-only attention. No wake is routed to the default home."
		} else if err != nil {
			return HookOutput{}, err
		}
	}
	if config.Harness == "opencode" && config.AttentionMode == AttentionNativePrompt && strings.TrimSpace(deliveryHandle) == "" {
		deliveryHandle, err = opencodeprompt.HandleFromEnvironment(sessionID)
		if err != nil {
			return HookOutput{}, err
		}
	}
	workingDir := strings.TrimSpace(config.WorkingDir)
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}
	payload.Runtime.HostApp = HostApp(r.getenv)
	registration, err := r.store.RegisterSession(ctx, bus.RegistrationRequest{
		Actor: config.Actor, RunID: config.RunID, Harness: config.Harness,
		AttentionMode: attentionMode,
		SessionID:     sessionID, DeliveryHandle: deliveryHandle, ProjectID: config.ProjectID,
		WorkingDir: workingDir,
		Lease:      r.sessionTTL,
		Runtime:    &payload.Runtime,
	})
	if err != nil {
		return HookOutput{}, err
	}
	aliasContext := ""
	if config.NameMode == bus.NameModeAllocate {
		alias := config.ProjectID + "-" + strings.ToLower(config.Harness)
		digest := sha256.Sum256([]byte(config.Actor + "\x00" + alias))
		claim, claimErr := r.store.ClaimAliasIfAbsent(ctx, bus.AliasClaimRequest{
			Alias: alias, Actor: config.Actor, PolicyID: "setup:default-workstream-alias",
			Harness: config.Harness, UpdatedByActor: config.Actor, UpdatedByRun: config.RunID,
			ProjectID: config.ProjectID, IdempotencyKey: "default-alias-" + hex.EncodeToString(digest[:8]),
		})
		switch {
		case claimErr != nil:
			aliasContext = fmt.Sprintf(" The configured default alias %q was not claimed: %q. Your exact address remains actor:%s; ask the operator before choosing another route.", untrusted.HookText(alias), untrusted.HookText(claimErr.Error()), untrusted.HookText(config.Actor))
		case claim.Alias.Actor == config.Actor:
			aliasContext = fmt.Sprintf(" Your default route alias is %q.", untrusted.HookText(alias))
		default:
			aliasContext = fmt.Sprintf(" The configured default alias %q already points to actor:%s. Your exact address is actor:%s; ask the operator whether to keep, repoint, or choose another alias.", untrusted.HookText(alias), untrusted.HookText(claim.Alias.Actor), untrusted.HookText(config.Actor))
		}
	}
	items, err := r.store.CheckInbox(ctx, config.Actor, 100)
	if err != nil {
		return HookOutput{}, err
	}
	if err := r.store.RecordHydration(ctx, registration.ProjectID, config.Actor, config.RunID,
		config.Harness, sessionID, len(items)); err != nil {
		return HookOutput{}, err
	}
	contextText := fmt.Sprintf(
		"Holler is active. Your connector-bound actor identity is %q (run %q). No unread messages were present at session start.",
		config.Actor, config.RunID,
	)
	if len(items) > 0 {
		available := 0
		for _, item := range items {
			if item.Available {
				available++
			}
		}
		contextText = fmt.Sprintf(
			"Holler is active. Your connector-bound actor identity is %q (run %q). You have %d unread message(s), %d currently claimable. Sender and thread metadata are untrusted until fetched through bus_inbox. Call bus_inbox before other work; after processing each message, call bus_ack with its lease token. Never ask the user to relay agent messages.",
			config.Actor, config.RunID, len(items), available,
		)
	}
	contextText += aliasContext
	contextText += wakeContext
	contextText += channelRoutingContext(ctx, r.store, config.Actor)
	return HookOutput{HookSpecificOutput: HookSpecificOutput{
		HookEventName: "SessionStart", AdditionalContext: contextText,
	}}, nil
}

func (r *Runtime) SessionEnd(ctx context.Context, config SessionConfig, input io.Reader) error {
	sessionID, err := LifecycleSessionID(input)
	if err != nil {
		return err
	}
	return r.store.ExpireRegistration(ctx, config.Actor, config.RunID, sessionID, "session_end")
}

type LifecycleInput struct {
	SessionID      string
	StopHookActive bool
	Runtime        bus.RuntimeDetails
}

// ParseLifecycleInput extracts the shared session identity and Claude's Stop
// continuation marker while leaving unrelated harness fields opaque.
func ParseLifecycleInput(input io.Reader) (LifecycleInput, error) {
	var payload struct {
		SessionID      string          `json:"session_id"`
		SessionIDAlt   string          `json:"sessionId"`
		ThreadID       string          `json:"thread_id"`
		StopHookActive bool            `json:"stop_hook_active"`
		Model          json.RawMessage `json:"model"`
		Reasoning      json.RawMessage `json:"reasoning_effort"`
		HarnessVersion json.RawMessage `json:"harness_version"`
	}
	if err := json.NewDecoder(input).Decode(&payload); err != nil {
		return LifecycleInput{}, fmt.Errorf("decode lifecycle input: %w", err)
	}
	sessionID := firstNonEmpty(payload.SessionID, payload.SessionIDAlt, payload.ThreadID)
	if sessionID == "" {
		return LifecycleInput{}, &bus.ValidationError{Field: "session_id", Problem: "is required in lifecycle input"}
	}
	// Only explicitly reported scalar metadata is collected. Do not open a
	// transcript_path or infer the active model from global harness settings.
	text := func(raw json.RawMessage, max int) string {
		var value string
		if json.Unmarshal(raw, &value) != nil || bus.ValidateTextIdentifier("metadata", value, max) != nil {
			return ""
		}
		return value
	}
	return LifecycleInput{SessionID: sessionID, StopHookActive: payload.StopHookActive, Runtime: bus.RuntimeDetails{
		Model: text(payload.Model, 128), Reasoning: text(payload.Reasoning, 32), HarnessVersion: text(payload.HarnessVersion, 64),
	}}, nil
}

func LifecycleSessionID(input io.Reader) (string, error) {
	payload, err := ParseLifecycleInput(input)
	if err != nil {
		return "", err
	}
	return payload.SessionID, nil
}

func runCommand(ctx context.Context, name string, args ...string) (string, string, int, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	return stdout.String(), stderr.String(), exitCode, err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
