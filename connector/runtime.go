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
	"sort"
	"strings"
	"time"

	"github.com/72olabs/holler-connectors/bus"
	"github.com/72olabs/holler-connectors/codexqueue"
	"github.com/72olabs/holler-connectors/opencodeprompt"
	"github.com/72olabs/holler-connectors/unread"
	"github.com/72olabs/holler-connectors/untrusted"
)

// Store is the part of the daemon's socket API the lifecycle hooks use;
// api.Client satisfies it.
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
// Runtime runs the lifecycle hooks: SessionStart registers the session and
// returns the start-up context, SessionEnd ends the registration.
type Runtime struct {
	store      Store
	sessionTTL time.Duration
	getenv     func(string) string
}

// CommandRunner runs a command and returns its output and exit code. Setup and
// Doctor take one so tests can replace the harness CLI.
type CommandRunner func(context.Context, string, ...string) (stdout, stderr string, exitCode int, err error)

// Option configures a Runtime.
type Option func(*Runtime)

// WithSessionTTL sets the registration lease SessionStart asks for (five minutes
// by default).
func WithSessionTTL(ttl time.Duration) Option { return func(r *Runtime) { r.sessionTTL = ttl } }

// WithEnvironment replaces the environment SessionStart reads the host app from.
func WithEnvironment(getenv func(string) string) Option {
	return func(r *Runtime) { r.getenv = getenv }
}

// New returns a Runtime that reaches the daemon through store.
func New(store Store, options ...Option) *Runtime {
	r := &Runtime{store: store, sessionTTL: 5 * time.Minute, getenv: os.Getenv}
	for _, option := range options {
		option(r)
	}
	return r
}

// SessionConfig is the identity and attention settings a hook registers or ends
// a session with.
type SessionConfig struct {
	Actor          string
	RunID          string
	Harness        string
	ProjectID      string
	AttentionMode  string
	DeliveryHandle string
	WorkingDir     string
	NameMode       bus.NameMode
	// AdoptedPredecessor is set when the daemon assigned a fresh actor
	// because the previous one was permanently adopted.
	AdoptedPredecessor string
}

// HookOutput is the JSON a SessionStart hook prints for the harness.
type HookOutput struct {
	HookSpecificOutput HookSpecificOutput `json:"hookSpecificOutput"`
}

// HookSpecificOutput carries the hook's additional context.
type HookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// SessionStart registers the session named in the hook input with the daemon
// and returns the start-up context the harness shows the agent: what is
// unread, the actor's aliases and conditions, within StartupContextBudget
// bytes. It lists messages and never claims them.
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
			wakeContext = "Live wake is unavailable (codex_home_invalid): new messages reach you only when you check."
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
	facts := StartupFacts{Actor: config.Actor, WakeNote: wakeContext, Predecessor: config.AdoptedPredecessor}
	defaultAlias, claimedDefault := "", false
	if config.NameMode == bus.NameModeAllocate {
		alias := config.ProjectID + "-" + strings.ToLower(config.Harness)
		defaultAlias = alias
		digest := sha256.Sum256([]byte(config.Actor + "\x00" + alias))
		claim, claimErr := r.store.ClaimAliasIfAbsent(ctx, bus.AliasClaimRequest{
			Alias: alias, Actor: config.Actor, PolicyID: "setup:default-workstream-alias",
			Harness: config.Harness, UpdatedByActor: config.Actor, UpdatedByRun: config.RunID,
			ProjectID: config.ProjectID, IdempotencyKey: "default-alias-" + hex.EncodeToString(digest[:8]),
		})
		quoted := untrusted.HookText(clip(alias, startupIDRunes))
		switch {
		case claimErr != nil:
			facts.AliasNote = fmt.Sprintf("Default alias %q was not assigned to you (%s).", quoted, untrusted.HookText(clip(claimErr.Error(), startupErrorRunes)))
		case claim.Alias.Actor == config.Actor:
			claimedDefault = true
		default:
			facts.AliasNote = fmt.Sprintf("Default alias %q was not assigned to you; it points to actor:%s. Use holler_alias_set only if the user asks for an alias.", quoted, untrusted.HookText(clip(claim.Alias.Actor, startupIDRunes)))
		}
	}
	if aliases, ok := actorAliases(ctx, r.store, config.Actor, defaultAlias); ok {
		facts.Aliases = aliases
		if len(aliases) > 0 {
			// The actor already answers to a name; the default alias
			// going elsewhere is no longer news.
			facts.AliasNote = ""
		}
	} else if claimedDefault {
		facts.Aliases = []string{defaultAlias}
	}
	reader, _ := r.store.(unread.CapabilityReader)
	var caps []bus.CapabilityDescriptor
	catalogRead := false
	if reader != nil {
		caps, err = reader.ListCapabilities(ctx)
		catalogRead = err == nil
	}
	facts.Counts, err = unread.CountWithCatalog(ctx, r.store, config.Actor, caps, catalogRead)
	if err != nil {
		return HookOutput{}, err
	}
	if err := r.store.RecordHydration(ctx, registration.ProjectID, config.Actor, config.RunID,
		config.Harness, sessionID, facts.Counts.Total()); err != nil {
		return HookOutput{}, err
	}
	if catalogRead {
		facts.Channels = readChannelDirectory(ctx, reader, caps)
	}
	presenter, conditions, more := ownConditions(ctx, r.store, config.Actor)
	facts.Conditions, facts.MoreConditions = conditions, more
	contextText := presentStartup(ctx, presenter, facts)
	return HookOutput{HookSpecificOutput: HookSpecificOutput{
		HookEventName: "SessionStart", AdditionalContext: contextText,
	}}, nil
}

// aliasLister is the alias listing of the daemon client.
type aliasLister interface {
	ListAliases(context.Context) ([]bus.ActorAlias, error)
}

// actorAliases returns the aliases that point at actor, the one to call it by
// first: aliases someone chose, most recently changed first, then the
// generated default alias. ok is false when aliases could not be listed.
func actorAliases(ctx context.Context, store Store, actor, defaultAlias string) ([]string, bool) {
	lister, ok := store.(aliasLister)
	if !ok {
		return nil, false
	}
	all, err := lister.ListAliases(ctx)
	if err != nil {
		return nil, false
	}
	var own []bus.ActorAlias
	seen := map[string]bool{}
	for _, a := range all {
		if a.Actor == actor && a.Alias != "" && !seen[a.Alias] {
			seen[a.Alias] = true
			own = append(own, a)
		}
	}
	sort.SliceStable(own, func(i, j int) bool {
		if (own[i].Alias == defaultAlias) != (own[j].Alias == defaultAlias) {
			return own[j].Alias == defaultAlias
		}
		return own[i].UpdatedAt.After(own[j].UpdatedAt)
	})
	names := make([]string, len(own))
	for i, a := range own {
		names[i] = a.Alias
	}
	return names, true
}

// SessionEnd ends the registration of the session named in the hook input.
func (r *Runtime) SessionEnd(ctx context.Context, config SessionConfig, input io.Reader) error {
	sessionID, err := LifecycleSessionID(input)
	if err != nil {
		return err
	}
	return r.store.ExpireRegistration(ctx, config.Actor, config.RunID, sessionID, "session_end")
}

// LifecycleInput is what the hooks read from a harness's hook input.
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

// LifecycleSessionID returns the session id from a harness's hook input.
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
