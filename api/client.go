package api

import (
	"bufio"
	"bytes"
	"context"

	"encoding/binary"

	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"

	"strings"
	"sync"

	"time"

	"github.com/72olabs/holler-connectors/buildinfo"
	"github.com/72olabs/holler-connectors/bus"
)

const (
	ProtocolVersion              = 1
	MaxFrameBytes                = 2 << 20
	MaxAttentionWait             = 25 * time.Second
	ActorAllocationCapability    = "actor-allocation-v1"
	ActorAliasCapability         = "actor-alias-v1"
	TypedRoutesCapability        = "typed-routes-v1"
	AliasClaimCapability         = "alias-claim-if-absent-v1"
	HarnessInstanceCapability    = "harness-instance-v1"
	HostAttentionCapability      = "host-attention-v1"
	OperatorConditionsCapability = "operator-conditions-v1"
	ActorLifecycleCapability     = "actor-lifecycle-v1"
	LegacyWakeCapability         = "legacy-wake-consumption-v1"
	CodexHookAttentionCapability = "codex-hook-attention-v1"
	// RuntimeHostAppCapability: the daemon accepts runtime.host_app in register_session.
	RuntimeHostAppCapability = "runtime-host-app-v1"
)

type Identity struct {
	DesktopHost        string
	Actor              string
	RunID              string
	Client             string
	Build              buildinfo.Info
	NameMode           bus.NameMode
	ContinuityHandles  []string
	ProjectID          string
	Harness            string
	HarnessInstance    string
	InstanceState      string
	Takeover           bool
	Provisional        bool
	AdoptedPredecessor string
	PendingPredecessor string
}

type Request struct {
	ID   uint64          `json:"id"`
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args,omitempty"`
}

type Response struct {
	ID     uint64          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type DaemonInfo struct {
	DatabaseSchema int            `json:"database_schema,omitempty"`
	Protocol       int            `json:"protocol"`
	Actor          string         `json:"actor"`
	PID            int            `json:"pid"`
	Build          buildinfo.Info `json:"build"`
	Capabilities   []string       `json:"capabilities,omitempty"`
}

type rpcResponseError struct{ err error }

func (e *rpcResponseError) Error() string { return e.err.Error() }
func (e *rpcResponseError) Unwrap() error { return e.err }

type identityReboundError struct{ validation *bus.ValidationError }

func (e *identityReboundError) Error() string { return e.validation.Error() }
func (e *identityReboundError) Unwrap() []error {
	return []error{e.validation, bus.ErrIdentityRebound}
}

type Client struct {
	connection         net.Conn
	connectionMu       sync.Mutex
	reader             *bufio.Reader
	socketPath         string
	identity           Identity
	helloIdentity      Identity
	serverBuild        buildinfo.Info
	serverCapabilities []string
	mu                 sync.Mutex
	nextID             uint64
}

func Dial(ctx context.Context, socketPath string, identity Identity) (*Client, error) {
	identity.Actor = strings.TrimSpace(identity.Actor)
	identity.RunID = strings.TrimSpace(identity.RunID)
	identity.Client = strings.TrimSpace(identity.Client)
	identity.ProjectID = strings.TrimSpace(identity.ProjectID)
	identity.Harness = strings.ToLower(strings.TrimSpace(identity.Harness))
	identity.NameMode = bus.NameMode(strings.TrimSpace(string(identity.NameMode)))
	if identity.Build.Commit == "" {
		identity.Build = buildinfo.Current()
	}
	client := &Client{socketPath: socketPath, identity: identity, helloIdentity: identity}
	bounded, cancel := withDefaultTimeout(ctx, 3*time.Second)
	defer cancel()
	client.mu.Lock()
	err := client.connectLocked(bounded)
	client.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (c *Client) connectLocked(ctx context.Context) error {
	includeProject := c.helloIdentity.ProjectID != ""
	err := c.connectAttemptLocked(ctx, true, includeProject)
	if err != nil && strings.Contains(err.Error(), `unknown field "build"`) {
		// Protocol v1 originally used strict hello decoding before build metadata
		// was added. Fall back without it so a new client can still operate during a
		// daemon-first rolling upgrade. The legacy daemon reports no build and
		// therefore cannot produce READY certification evidence.
		err = c.connectAttemptLocked(ctx, false, includeProject)
	}
	featureIdentity := c.helloIdentity.NameMode != "" || len(c.helloIdentity.ContinuityHandles) > 0 || c.helloIdentity.Takeover
	if err != nil && includeProject && !featureIdentity && strings.Contains(err.Error(), `unknown field "project_id"`) {
		// Project identity became part of protocol v1 after the oldest strict
		// daemon. That daemon has no capability bridge, so legacy sessions may
		// omit project context only to preserve their pre-bridge operations.
		return c.connectAttemptLocked(ctx, false, false)
	}
	return err
}

func (c *Client) connectAttemptLocked(ctx context.Context, includeBuild, includeProject bool) error {
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("connect to hollerd at %s: %w", c.socketPath, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	c.connectionMu.Lock()
	c.connection = connection
	c.connectionMu.Unlock()
	c.reader = bufio.NewReader(connection)
	c.nextID = 1
	hello := map[string]interface{}{
		"protocol": ProtocolVersion, "client": c.helloIdentity.Client,
		"actor": c.helloIdentity.Actor, "run_id": c.helloIdentity.RunID,
	}
	featureIdentity := c.helloIdentity.NameMode != "" || len(c.helloIdentity.ContinuityHandles) > 0 || c.helloIdentity.Takeover
	featureHarness := c.helloIdentity.Harness != ""
	if c.helloIdentity.DesktopHost != "" {
		hello["desktop_host"] = c.helloIdentity.DesktopHost
	}
	if includeProject {
		hello["project_id"] = c.helloIdentity.ProjectID
	}
	if featureIdentity || featureHarness {
		hello["capabilities"] = []string{ActorAllocationCapability, ActorAliasCapability, TypedRoutesCapability, AliasClaimCapability, HarnessInstanceCapability, OperatorConditionsCapability, ActorLifecycleCapability, ManagedConversationsCapability}
	}
	if !featureIdentity && !featureHarness && strings.HasPrefix(c.helloIdentity.Client, bus.ServiceClientPrefix) {
		// An agent service holds an existing actor with no harness of its own;
		// it negotiates only the channel wakes its parked wait takes.
		hello["capabilities"] = []string{ManagedConversationsCapability}
	}
	if strings.HasPrefix(c.helloIdentity.Client, "bus-mcp/") {
		caps, _ := hello["capabilities"].([]string)
		hello["capabilities"] = append(caps, LegacyWakeCapability)
	}
	if featureIdentity {
		hello["name_mode"] = c.helloIdentity.NameMode
		hello["continuity_handles"] = c.helloIdentity.ContinuityHandles
		hello["takeover"] = c.helloIdentity.Takeover
	}
	if featureHarness {
		hello["harness"] = c.helloIdentity.Harness
	}
	if includeBuild {
		hello["build"] = c.helloIdentity.Build
	}
	request := Request{ID: c.nextID, Op: "hello"}
	request.Args, err = json.Marshal(hello)
	if err == nil {
		err = writeRequest(connection, request)
	}
	var response Response
	if err == nil {
		response, err = readResponse(c.reader)
	}
	_ = connection.SetDeadline(time.Time{})
	if err != nil {
		c.closeLocked()
		return fmt.Errorf("hollerd hello: %w", err)
	}
	if response.ID != request.ID || !response.OK {
		c.closeLocked()
		if response.Error != nil {
			return errorFromRPC(response.Error)
		}
		return errors.New("hollerd returned invalid hello response")
	}
	var ready struct {
		Protocol           int            `json:"protocol"`
		Actor              string         `json:"actor"`
		RunID              string         `json:"run_id"`
		Build              buildinfo.Info `json:"build"`
		Capabilities       []string       `json:"capabilities"`
		Provisional        bool           `json:"provisional"`
		AdoptedPredecessor string         `json:"adopted_predecessor"`
		PendingPredecessor string         `json:"pending_predecessor"`
		HarnessInstance    string         `json:"harness_instance"`
		InstanceState      string         `json:"instance_state"`
	}
	if err := json.Unmarshal(response.Result, &ready); err != nil {
		c.closeLocked()
		return fmt.Errorf("decode hollerd hello: %w", err)
	}
	if ready.RunID == "" && !featureIdentity && !featureHarness {
		// The oldest protocol-v1 daemon did not echo run_id in READY. This is
		// safe only for the legacy path, where no daemon-owned identity binding
		// was negotiated and the authenticated run therefore cannot change.
		ready.RunID = c.helloIdentity.RunID
	}
	if ready.Protocol != ProtocolVersion || ready.Actor == "" || ready.RunID == "" || (!featureIdentity && ready.Actor != c.helloIdentity.Actor) {
		c.closeLocked()
		return errors.New("hollerd returned mismatched session identity")
	}
	if featureIdentity && !containsString(ready.Capabilities, ActorAllocationCapability) {
		c.closeLocked()
		return errors.New("hollerd does not support negotiated actor allocation")
	}
	c.identity.Actor = ready.Actor
	c.identity.RunID = ready.RunID
	c.identity.Provisional = ready.Provisional
	c.identity.AdoptedPredecessor = ready.AdoptedPredecessor
	c.identity.PendingPredecessor = ready.PendingPredecessor
	c.identity.HarnessInstance = ready.HarnessInstance
	c.identity.InstanceState = ready.InstanceState
	c.serverBuild = ready.Build
	c.serverCapabilities = append([]string(nil), ready.Capabilities...)
	return nil
}

func (c *Client) Identity() Identity {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.identity
}

func (c *Client) BoundIdentity() (string, string) {
	identity := c.Identity()
	return identity.Actor, identity.RunID
}

func (c *Client) closeLocked() {
	c.connectionMu.Lock()
	defer c.connectionMu.Unlock()
	if c.connection != nil {
		_ = c.connection.Close()
	}
	c.connection = nil
	c.reader = nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
	return nil
}

// Interrupt closes the current transport without waiting for an in-flight
// long poll. The next API call reconnects normally.
func (c *Client) Interrupt() {
	c.connectionMu.Lock()
	defer c.connectionMu.Unlock()
	if c.connection != nil {
		_ = c.connection.Close()
	}
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.DaemonInfo(ctx)
	return err
}

func (c *Client) DaemonInfo(ctx context.Context) (DaemonInfo, error) {
	var result DaemonInfo
	err := c.call(ctx, "ping", struct{}{}, &result)
	return result, err
}

func (c *Client) ServerBuild() buildinfo.Info {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.serverBuild
}

func (c *Client) Send(ctx context.Context, request bus.SendRequest) (bus.SendResult, error) {
	if (len(request.Destinations) > 0 || (strings.TrimSpace(request.InReplyTo) != "" && len(request.ToActors) == 0)) &&
		!c.supports(TypedRoutesCapability) {
		return bus.SendResult{}, &bus.ValidationError{Field: "destinations", Problem: "connected daemon does not support typed routes; upgrade hollerd"}
	}
	request.FromActor = ""
	request.FromRun = ""
	var result bus.SendResult
	err := c.call(ctx, "send", request, &result)
	return result, err
}

func (c *Client) CheckInbox(ctx context.Context, actor string, limit int) ([]bus.InboxItem, error) {
	if err := c.requireActor(actor); err != nil {
		return nil, err
	}
	var result []bus.InboxItem
	err := c.call(ctx, "check_inbox", map[string]interface{}{"limit": limit}, &result)
	return result, err
}

func (c *Client) WaitAttention(ctx context.Context, actor, runID, sessionID, adapter string, wait time.Duration) (bus.AttentionNotice, error) {
	if err := c.requireActor(actor); err != nil {
		return bus.AttentionNotice{}, err
	}
	if err := c.requireRun(runID); err != nil {
		return bus.AttentionNotice{}, err
	}
	waitCtx, cancel := withDefaultTimeout(ctx, wait+2*time.Second)
	defer cancel()
	var notice bus.AttentionNotice
	err := c.call(waitCtx, "wait_attention", map[string]interface{}{
		"session_id": sessionID, "adapter": adapter, "wait_ns": int64(wait),
	}, &notice)
	return notice, err
}

func (c *Client) MonitorAttach(ctx context.Context, actor, runID, sessionID, adapter string, lease time.Duration) (bus.Registration, error) {
	if err := c.requireActor(actor); err != nil {
		return bus.Registration{}, err
	}
	if err := c.requireRun(runID); err != nil {
		return bus.Registration{}, err
	}
	var registration bus.Registration
	err := c.call(ctx, "monitor_attach", map[string]interface{}{
		"session_id": sessionID, "adapter": adapter, "lease_ns": int64(lease),
	}, &registration)
	return registration, err
}

func (c *Client) Claim(ctx context.Context, actor, messageID string, lease time.Duration) (bus.Claim, error) {
	if err := c.requireActor(actor); err != nil {
		return bus.Claim{}, err
	}
	var result bus.Claim
	err := c.call(ctx, "claim", map[string]interface{}{"message_id": messageID, "lease_ns": int64(lease)}, &result)
	return result, err
}

func (c *Client) Ack(ctx context.Context, actor, messageID, leaseToken string) error {
	if err := c.requireActor(actor); err != nil {
		return err
	}
	return c.call(ctx, "ack", map[string]interface{}{"message_id": messageID, "lease_token": leaseToken}, &struct{}{})
}

func (c *Client) Extend(ctx context.Context, actor, messageID, leaseToken string, lease time.Duration) (bus.LeaseExtension, error) {
	if err := c.requireActor(actor); err != nil {
		return bus.LeaseExtension{}, err
	}
	var result bus.LeaseExtension
	err := c.call(ctx, "extend", map[string]interface{}{
		"message_id": messageID, "lease_token": leaseToken, "lease_ns": int64(lease),
	}, &result)
	return result, err
}

func (c *Client) Nack(ctx context.Context, actor, messageID, leaseToken, reason string, final bool) error {
	if err := c.requireActor(actor); err != nil {
		return err
	}
	return c.call(ctx, "nack", map[string]interface{}{
		"message_id": messageID, "lease_token": leaseToken, "reason": reason, "final": final,
	}, &struct{}{})
}

func (c *Client) ListEvents(ctx context.Context, partition, stream string, after int64, limit int) ([]bus.Event, error) {
	var result []bus.Event
	err := c.call(ctx, "list_events", map[string]interface{}{
		"partition": partition, "stream": stream, "after": after, "limit": limit,
	}, &result)
	return result, err
}

func (c *Client) SetActorProfile(ctx context.Context, actor, runID, projectID string, request bus.ActorProfileRequest) (bus.ActorProfileResult, error) {
	if err := c.requireActor(actor); err != nil {
		return bus.ActorProfileResult{}, err
	}
	if err := c.requireRun(runID); err != nil {
		return bus.ActorProfileResult{}, err
	}
	var result bus.ActorProfileResult
	err := c.call(ctx, "set_actor_profile", map[string]interface{}{
		"project_id": projectID, "role_text": request.RoleText, "accepts": request.Accepts,
	}, &result)
	return result, err
}

func (c *Client) Who(ctx context.Context, limit int) (bus.ActorDirectory, error) {
	var result bus.ActorDirectory
	err := c.call(ctx, "who", map[string]interface{}{"limit": limit, "include_archived": false}, &result)
	return result, err
}

func (c *Client) WhoIncludingArchived(ctx context.Context, limit int) (bus.ActorDirectory, error) {
	var result bus.ActorDirectory
	err := c.call(ctx, "who", map[string]interface{}{"limit": limit, "include_archived": true}, &result)
	return result, err
}

func (c *Client) ArchivePreflight(ctx context.Context, actor string, limit int) (bus.ActorArchivePreflight, error) {
	var result bus.ActorArchivePreflight
	err := c.call(ctx, "archive_preflight", map[string]interface{}{"actor": actor, "limit": limit}, &result)
	return result, err
}

func (c *Client) ArchiveActor(ctx context.Context, actor string, allowUnread bool) (bus.ActorArchiveResult, error) {
	var result bus.ActorArchiveResult
	err := c.call(ctx, "archive_actor", map[string]interface{}{"actor": actor, "allow_unread": allowUnread}, &result)
	return result, err
}

func (c *Client) RestoreActor(ctx context.Context, actor string) (bus.ActorArchiveResult, error) {
	var result bus.ActorArchiveResult
	err := c.call(ctx, "restore_actor", map[string]interface{}{"actor": actor}, &result)
	return result, err
}

func (c *Client) RevokeDeliveryLease(ctx context.Context, actor, messageID string, crashGrace time.Duration) error {
	return c.call(ctx, "revoke_delivery_lease", map[string]interface{}{
		"actor": actor, "message_id": messageID, "crash_grace_ns": int64(crashGrace),
	}, &struct{}{})
}

func (c *Client) AdoptActor(ctx context.Context, request bus.AdoptRequest) (bus.AdoptResult, error) {
	var result bus.AdoptResult
	err := c.call(ctx, "adopt_actor", map[string]interface{}{
		"source_actor": request.SourceActor, "project_id": request.ProjectID, "idempotency_key": request.IdempotencyKey,
	}, &result)
	return result, err
}

func (c *Client) SetAlias(ctx context.Context, request bus.AliasSetRequest) (bus.AliasMutationResult, error) {
	var result bus.AliasMutationResult
	err := c.call(ctx, "set_alias", map[string]interface{}{
		"alias": request.Alias, "actor": request.Actor, "project_id": request.ProjectID,
		"idempotency_key": request.IdempotencyKey,
	}, &result)
	return result, err
}

func (c *Client) ClaimAliasIfAbsent(ctx context.Context, request bus.AliasClaimRequest) (bus.AliasClaimResult, error) {
	if !c.supports(AliasClaimCapability) {
		return bus.AliasClaimResult{}, &bus.ValidationError{Field: "policy_id", Problem: "connected daemon does not support atomic alias claims; upgrade hollerd"}
	}
	var result bus.AliasClaimResult
	err := c.call(ctx, "claim_alias_if_absent", map[string]interface{}{
		"alias": request.Alias, "actor": request.Actor, "policy_id": request.PolicyID, "harness": request.Harness,
		"project_id": request.ProjectID, "idempotency_key": request.IdempotencyKey,
	}, &result)
	return result, err
}

// ensureConnected completes the hello, so supports reflects this daemon.
func (c *Client) ensureConnected(ctx context.Context) error {
	ctx, cancel := withDefaultTimeout(ctx, 5*time.Second)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connection == nil {
		return c.connectLocked(ctx)
	}
	return nil
}

func (c *Client) supports(capability string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return containsString(c.serverCapabilities, capability)
}

// Supports reports whether the connected daemon advertised capability in its
// hello, so a newer client never relies on an older daemon for it.
func (c *Client) Supports(ctx context.Context, capability string) (bool, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return false, err
	}
	return c.supports(capability), nil
}

func (c *Client) RemoveAlias(ctx context.Context, request bus.AliasRemoveRequest) (bus.AliasMutationResult, error) {
	var result bus.AliasMutationResult
	err := c.call(ctx, "remove_alias", map[string]interface{}{
		"alias": request.Alias, "project_id": request.ProjectID, "idempotency_key": request.IdempotencyKey,
	}, &result)
	return result, err
}

func (c *Client) ListAliases(ctx context.Context) ([]bus.ActorAlias, error) {
	var result []bus.ActorAlias
	err := c.call(ctx, "list_aliases", struct{}{}, &result)
	return result, err
}

func (c *Client) ResolveAlias(ctx context.Context, alias string) (bus.ActorAlias, error) {
	var result bus.ActorAlias
	err := c.call(ctx, "resolve_alias", map[string]interface{}{"alias": alias}, &result)
	return result, err
}

func (c *Client) AliasPreflight(ctx context.Context, alias, proposedActor string) (bus.AliasPreflight, error) {
	var result bus.AliasPreflight
	err := c.call(ctx, "alias_preflight", map[string]interface{}{"alias": alias, "proposed_actor": proposedActor}, &result)
	return result, err
}

func (c *Client) ListCapabilities(ctx context.Context) ([]bus.CapabilityDescriptor, error) {
	var result []bus.CapabilityDescriptor
	err := c.call(ctx, "list_capabilities", struct{}{}, &result)
	return result, err
}

func (c *Client) ListConditions(ctx context.Context, includeResolved bool, limit int) ([]bus.OperatorCondition, error) {
	var result []bus.OperatorCondition
	err := c.call(ctx, "list_conditions", map[string]interface{}{
		"include_resolved": includeResolved, "limit": limit,
	}, &result)
	return result, err
}

func (c *Client) AcknowledgeCondition(ctx context.Context, kind, subject string, generation int) (bus.OperatorCondition, error) {
	var result bus.OperatorCondition
	err := c.call(ctx, "acknowledge_condition", map[string]interface{}{
		"kind": kind, "subject": subject, "generation": generation,
	}, &result)
	return result, err
}

func (c *Client) SnoozeCondition(ctx context.Context, kind, subject string, generation int, until time.Time) (bus.OperatorCondition, error) {
	var result bus.OperatorCondition
	err := c.call(ctx, "snooze_condition", map[string]interface{}{
		"kind": kind, "subject": subject, "generation": generation, "until": until,
	}, &result)
	return result, err
}

func (c *Client) ClaimConditionPresentation(ctx context.Context, kind, subject string, generation int, lease time.Duration) (bool, error) {
	var result struct {
		Claimed bool `json:"claimed"`
	}
	err := c.call(ctx, "claim_condition_presentation", map[string]interface{}{
		"kind": kind, "subject": subject, "generation": generation, "lease_ns": int64(lease),
	}, &result)
	return result.Claimed, err
}

func (c *Client) InvokeReadCapability(ctx context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
	return c.invokeCapability(ctx, "invoke_read_capability", name, arguments)
}

func (c *Client) InvokeWriteCapability(ctx context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
	return c.invokeCapability(ctx, "invoke_write_capability", name, arguments)
}

func (c *Client) invokeCapability(ctx context.Context, operation, name string, arguments json.RawMessage) (json.RawMessage, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	invocation := bus.CapabilityInvocation{Name: name, Arguments: arguments}
	if err := bus.ValidateCapabilityInvocation(invocation); err != nil {
		return nil, err
	}
	var result json.RawMessage
	err := c.call(ctx, operation, invocation, &result)
	return result, err
}

func (c *Client) RecordAgentClient(ctx context.Context, actor, run string, client bus.AgentClient) error {
	if err := c.requireActor(actor); err != nil {
		return err
	}
	return c.call(ctx, "record_agent_client", client, &struct{}{})
}

func (c *Client) RegisterSession(ctx context.Context, request bus.RegistrationRequest) (bus.Registration, error) {
	if err := c.requireActor(request.Actor); err != nil {
		return bus.Registration{}, err
	}
	request.Actor, request.RunID = "", ""
	// Older daemons decode strictly and would refuse the whole registration over
	// host_app, so it is sent only to a daemon that says it accepts it.
	if request.Runtime != nil && request.Runtime.HostApp != "" {
		if err := c.ensureConnected(ctx); err != nil {
			return bus.Registration{}, err
		}
		if !c.supports(RuntimeHostAppCapability) {
			runtime := *request.Runtime
			runtime.HostApp = ""
			request.Runtime = &runtime
		}
	}
	var result bus.Registration
	err := c.call(ctx, "register_session", request, &result)
	return result, err
}

func (c *Client) LiveRegistrations(ctx context.Context, actor string) ([]bus.Registration, error) {
	if err := c.requireActor(actor); err != nil {
		return nil, err
	}
	var result []bus.Registration
	err := c.call(ctx, "live_registrations", map[string]interface{}{"actor": actor}, &result)
	return result, err
}

func (c *Client) RecordHydration(ctx context.Context, projectID, actor, runID, harness, sessionID string, unread int) error {
	if err := c.requireActor(actor); err != nil {
		return err
	}
	if err := c.requireRun(runID); err != nil {
		return err
	}
	return c.call(ctx, "record_hydration", map[string]interface{}{
		"project_id": projectID, "run_id": runID, "harness": harness, "session_id": sessionID, "unread": unread,
	}, &struct{}{})
}

func (c *Client) ExpireRegistration(ctx context.Context, actor, runID, sessionID, reason string) error {
	if err := c.requireActor(actor); err != nil {
		return err
	}
	if err := c.requireRun(runID); err != nil {
		return err
	}
	return c.call(ctx, "expire_registration", map[string]interface{}{
		"session_id": sessionID, "reason": reason,
	}, &struct{}{})
}

func (c *Client) HeartbeatRegistrations(ctx context.Context, actor, runID string, lease time.Duration) (int, error) {
	if err := c.requireActor(actor); err != nil {
		return 0, err
	}
	if err := c.requireRun(runID); err != nil {
		return 0, err
	}
	var result struct {
		Renewed int `json:"renewed"`
	}
	err := c.call(ctx, "heartbeat_registrations", map[string]interface{}{"lease_ns": int64(lease)}, &result)
	return result.Renewed, err
}

func (c *Client) requireActor(actor string) error {
	identity := c.Identity()
	if strings.TrimSpace(actor) != identity.Actor {
		return &identityReboundError{validation: &bus.ValidationError{Field: "actor", Problem: "does not match the authenticated API session"}}
	}
	return nil
}

func (c *Client) requireRun(runID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	candidate := strings.TrimSpace(runID)
	if candidate != c.identity.RunID && candidate != c.helloIdentity.RunID {
		return &identityReboundError{validation: &bus.ValidationError{Field: "run_id", Problem: "does not match the authenticated API session"}}
	}
	return nil
}

func (c *Client) call(ctx context.Context, op string, args interface{}, target interface{}) error {
	ctx, cancel := withDefaultTimeout(ctx, 5*time.Second)
	defer cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connection == nil {
		if err := c.connectLocked(ctx); err != nil {
			return err
		}
	}
	err := c.callOnceLocked(ctx, op, args, target)
	if err == nil {
		if c.identity.Provisional && finalizesProvisionalActor(op) {
			c.identity.Provisional = false
		}
		return nil
	}
	var responseErr *rpcResponseError
	if errors.As(err, &responseErr) {
		if errors.Is(err, bus.ErrBindingReassigned) && ctx.Err() == nil {
			c.closeLocked()
			if reconnectErr := c.connectLocked(ctx); reconnectErr != nil {
				return reconnectErr
			}
			retryErr := c.callOnceLocked(ctx, op, args, target)
			if retryErr == nil && c.identity.Provisional && finalizesProvisionalActor(op) {
				c.identity.Provisional = false
			}
			return retryErr
		}
		return err
	}
	c.closeLocked()
	if !retryAfterReconnect(op) || ctx.Err() != nil {
		return err
	}
	if err := c.connectLocked(ctx); err != nil {
		return err
	}
	retryErr := c.callOnceLocked(ctx, op, args, target)
	if retryErr == nil && c.identity.Provisional && finalizesProvisionalActor(op) {
		c.identity.Provisional = false
	}
	return retryErr
}

func (c *Client) callOnceLocked(ctx context.Context, op string, args interface{}, target interface{}) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.connection.SetDeadline(deadline)
		defer c.connection.SetDeadline(time.Time{})
	}
	c.nextID++
	raw, err := json.Marshal(args)
	if err != nil {
		return err
	}
	if err := writeRequest(c.connection, Request{ID: c.nextID, Op: op, Args: raw}); err != nil {
		return fmt.Errorf("write %s request: %w", op, err)
	}
	response, err := readResponse(c.reader)
	if err != nil {
		return fmt.Errorf("read %s response: %w", op, err)
	}
	if response.ID != c.nextID {
		return errors.New("hollerd returned mismatched request id")
	}
	if !response.OK {
		return &rpcResponseError{err: errorFromRPC(response.Error)}
	}
	if target == nil || len(response.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Result, target); err != nil {
		return fmt.Errorf("decode %s result: %w", op, err)
	}
	return nil
}

func retryAfterReconnect(op string) bool {
	switch op {
	case "record_agent_client":
		return true
	case "ping", "send", "check_inbox", "ack", "extend", "list_events", "who", "set_alias", "remove_alias", "list_aliases", "resolve_alias", "alias_preflight", "list_capabilities", "invoke_read_capability", "live_registrations", "monitor_attach", "expire_registration", "heartbeat_registrations", "list_conditions", "claim_condition_presentation", "archive_preflight":
		return true
	default:
		return false
	}
}

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func writeRequest(writer io.Writer, request Request) error    { return writeFrame(writer, request) }
func writeResponse(writer io.Writer, response Response) error { return writeFrame(writer, response) }

func writeFrame(writer io.Writer, value interface{}) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) > MaxFrameBytes {
		return errors.New("API frame exceeds 2 MiB")
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	if err := writeAll(writer, length[:]); err != nil {
		return err
	}
	return writeAll(writer, payload)
}

func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}

func readRequest(reader io.Reader) (Request, error) {
	var request Request
	err := readFrame(reader, &request)
	return request, err
}

func readResponse(reader io.Reader) (Response, error) {
	var response Response
	err := readFrame(reader, &response)
	return response, err
}

func readFrame(reader io.Reader, target interface{}) error {
	var length [4]byte
	if _, err := io.ReadFull(reader, length[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(length[:])
	if size == 0 || size > MaxFrameBytes {
		return fmt.Errorf("invalid API frame length %d", size)
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return err
	}
	return decodeStrict(payload, target)
}

func decodeStrict(raw []byte, target interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func failure(id uint64, code, message string, retryable bool) Response {
	return Response{ID: id, OK: false, Error: &RPCError{Code: code, Message: message, Retryable: retryable}}
}

// ErrorDetails is the shared wire-code contract for Unix RPC and the local
// human gateway. Transports may choose status codes, not redefine error names.
func ErrorDetails(err error) *RPCError { return rpcError(err) }

func rpcError(err error) *RPCError {
	code := "internal"
	switch {
	case errors.Is(err, bus.ErrChannelDenied):
		code = "conversation_denied"
	case errors.Is(err, bus.ErrPreflightExpired):
		code = "preflight_expired"
	case errors.Is(err, bus.ErrCursorExpired):
		code = "cursor_expired"
	case errors.Is(err, bus.ErrAudienceChanged):
		code = "audience_changed"
	case errors.Is(err, bus.ErrImmutableAudience):
		code = "immutable_audience"
	case errors.Is(err, bus.ErrChannelCapability):
		code = "capability_required"
	case errors.Is(err, bus.ErrShareAuthority):
		code = "share_authority_required"
	case errors.Is(err, bus.ErrResponseConflict):
		code = "response_conflict"
	case errors.Is(err, bus.ErrAttentionConflict):
		code = "attention_conflict"
	case errors.Is(err, bus.ErrAttentionLocked):
		code = "attention_locked"
	case errors.Is(err, bus.ErrThreadObligation):
		code = "thread_obligation"
	case errors.Is(err, bus.ErrConfirmationNotRevisable):
		code = "confirmation_not_revisable"
	case errors.Is(err, bus.ErrTooManyRevisions):
		code = "too_many_revisions"
	case errors.Is(err, bus.ErrMemberPostNotEligible):
		code = "member_post_not_eligible"
	case errors.Is(err, bus.ErrHumanTurnRequired):
		code = "human_turn_required"
	case errors.Is(err, bus.ErrTooManyOpenRequests):
		code = "too_many_open_requests"
	case errors.Is(err, bus.ErrInvalid):
		code = "invalid_request"
	case errors.Is(err, bus.ErrNotFound):
		code = "not_found"
	case errors.Is(err, bus.ErrNoMessage):
		code = "no_message"
	case errors.Is(err, bus.ErrAttentionWaiterBusy):
		code = "attention_waiter_busy"
	case errors.Is(err, bus.ErrAttentionUnavailable):
		code = "attention_unavailable"
	case errors.Is(err, bus.ErrSessionEnded):
		code = "session_ended"
	case errors.Is(err, bus.ErrPresenceSuperseded):
		code = "presence_superseded"
	case errors.Is(err, bus.ErrRegistrationExpired):
		return &RPCError{Code: "registration_expired", Message: err.Error(), Retryable: true}
	case errors.Is(err, bus.ErrIdempotencyConflict):
		code = "idempotency_conflict"
	case errors.Is(err, bus.ErrLeaseTokenMismatch):
		code = "lease_token_mismatch"
	case errors.Is(err, bus.ErrLeaseExpired):
		code = "lease_expired"
	case errors.Is(err, bus.ErrDeliveryTerminal):
		code = "delivery_terminal"
	case errors.Is(err, bus.ErrActorLive):
		code = "actor_live"
	case errors.Is(err, bus.ErrActorArchived):
		code = "actor_archived"
	case errors.Is(err, bus.ErrBindingStale):
		code = "binding_stale"
	case errors.Is(err, bus.ErrContinuityConflict):
		code = "continuity_conflict"
	case errors.Is(err, bus.ErrBindingReassigned):
		return &RPCError{Code: "binding_reassigned", Message: err.Error(), Retryable: true}
	case errors.Is(err, bus.ErrAdoptionConflict):
		code = "adoption_conflict"
	case errors.Is(err, bus.ErrAdoptionBusy):
		code = "adoption_busy"
	case errors.Is(err, bus.ErrActorNotLive):
		code = "actor_not_live"
	case errors.Is(err, bus.ErrRunNotLive):
		code = "run_not_live"
	case errors.Is(err, bus.ErrActorAdopted):
		code = "actor_adopted"
	case errors.Is(err, bus.ErrAliasConflict):
		code = "alias_conflict"
	case errors.Is(err, bus.ErrAliasNotFound):
		code = "alias_not_found"
	case errors.Is(err, bus.ErrAliasTombstoned):
		code = "alias_tombstoned"
	case errors.Is(err, bus.ErrAliasTargetUnknown):
		code = "alias_target_unknown"
	}
	return &RPCError{Code: code, Message: err.Error(), Retryable: false}
}

func errorFromRPC(rpc *RPCError) error {
	if rpc == nil {
		return errors.New("hollerd returned an empty error")
	}
	var sentinel error
	switch rpc.Code {
	case "conversation_denied":
		sentinel = bus.ErrChannelDenied
	case "preflight_expired":
		sentinel = bus.ErrPreflightExpired
	case "cursor_expired":
		sentinel = bus.ErrCursorExpired
	case "audience_changed":
		sentinel = bus.ErrAudienceChanged
	case "immutable_audience":
		sentinel = bus.ErrImmutableAudience
	case "capability_required":
		sentinel = bus.ErrChannelCapability
	case "share_authority_required":
		sentinel = bus.ErrShareAuthority
	case "response_conflict":
		sentinel = bus.ErrResponseConflict
	case "attention_conflict":
		sentinel = bus.ErrAttentionConflict
	case "attention_locked":
		sentinel = bus.ErrAttentionLocked
	case "thread_obligation":
		sentinel = bus.ErrThreadObligation
	case "confirmation_not_revisable":
		sentinel = bus.ErrConfirmationNotRevisable
	case "too_many_revisions":
		sentinel = bus.ErrTooManyRevisions
	case "member_post_not_eligible":
		sentinel = bus.ErrMemberPostNotEligible
	case "human_turn_required":
		sentinel = bus.ErrHumanTurnRequired
	case "too_many_open_requests":
		sentinel = bus.ErrTooManyOpenRequests
	case "invalid_request":
		sentinel = bus.ErrInvalid
	case "not_found":
		sentinel = bus.ErrNotFound
	case "no_message":
		sentinel = bus.ErrNoMessage
	case "attention_waiter_busy":
		sentinel = bus.ErrAttentionWaiterBusy
	case "attention_unavailable":
		sentinel = bus.ErrAttentionUnavailable
	case "session_ended":
		sentinel = bus.ErrSessionEnded
	case "presence_superseded":
		sentinel = bus.ErrPresenceSuperseded
	case "registration_expired":
		sentinel = bus.ErrRegistrationExpired
	case "idempotency_conflict":
		sentinel = bus.ErrIdempotencyConflict
	case "lease_token_mismatch":
		sentinel = bus.ErrLeaseTokenMismatch
	case "lease_expired":
		sentinel = bus.ErrLeaseExpired
	case "delivery_terminal":
		sentinel = bus.ErrDeliveryTerminal
	case "actor_live":
		sentinel = bus.ErrActorLive
	case "actor_archived":
		sentinel = bus.ErrActorArchived
	case "binding_stale":
		sentinel = bus.ErrBindingStale
	case "continuity_conflict":
		sentinel = bus.ErrContinuityConflict
	case "binding_reassigned":
		sentinel = bus.ErrBindingReassigned
	case "adoption_conflict":
		sentinel = bus.ErrAdoptionConflict
	case "adoption_busy":
		sentinel = bus.ErrAdoptionBusy
	case "actor_not_live":
		sentinel = bus.ErrActorNotLive
	case "run_not_live":
		sentinel = bus.ErrRunNotLive
	case "actor_adopted":
		sentinel = bus.ErrActorAdopted
	case "alias_conflict":
		sentinel = bus.ErrAliasConflict
	case "alias_not_found":
		sentinel = bus.ErrAliasNotFound
	case "alias_tombstoned":
		sentinel = bus.ErrAliasTombstoned
	case "alias_target_unknown":
		sentinel = bus.ErrAliasTargetUnknown
	}
	if sentinel != nil {
		text := sentinel.Error()
		if rpc.Message == "" || rpc.Message == text {
			return sentinel
		}
		if strings.HasSuffix(rpc.Message, ": "+text) {
			return fmt.Errorf("%s: %w", strings.TrimSuffix(rpc.Message, ": "+text), sentinel)
		}
		return fmt.Errorf("%s: %w", rpc.Message, sentinel)
	}
	return errors.New(rpc.Message)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (c *Client) ConsumeLegacyWake(ctx context.Context, actor, runID, wakeID string) (bool, error) {
	if err := c.requireActor(actor); err != nil {
		return false, err
	}
	if err := c.requireRun(runID); err != nil {
		return false, err
	}
	var consumed bool
	err := c.call(ctx, "consume_legacy_wake", map[string]string{"wake_id": wakeID}, &consumed)
	return consumed, err
}

// PollCodexHook requires a new daemon; older daemons remain queue-only.
func (c *Client) PollCodexHook(ctx context.Context, sessionID, hookEvent string) (bus.CodexHookNotice, error) {
	if !c.supports(CodexHookAttentionCapability) {
		return bus.CodexHookNotice{}, bus.ErrAttentionUnavailable
	}
	var out bus.CodexHookNotice
	err := c.call(ctx, "poll_codex_hook", map[string]interface{}{"session_id": sessionID, "hook_event": hookEvent}, &out)
	return out, err
}

func finalizesProvisionalActor(operation string) bool {
	switch operation {
	case "record_agent_client":
		return false
	case "ping", "list_events", "who", "list_aliases", "resolve_alias", "list_capabilities", "invoke_read_capability", "live_registrations", "heartbeat_registrations", "list_conditions":
		return false
	default:
		return true
	}
}
