# Compatibility and ownership

## Daemon wire protocol

`api/` is a client implementation, not the daemon server. It connects through a
Unix socket, sending JSON frames prefixed by a 4-byte big-endian payload length.
The frame limit is 2 MiB. `hello` negotiates protocol version 1, identity, build
information, and capabilities. `bus/` contains shared wire types and validation;
`buildinfo/` supplies build identity. These Go APIs are initially experimental;
the wire contract, not Go type layout, is the compatibility boundary.

A matching protocol number is necessary but not sufficient. Optional features
are capability-gated; unknown operations must not be interpreted as success.
The client preserves the existing legacy hello fallback but never downgrades a
negotiated allocation request. Tests cover that boundary. `connector doctor`
checks configuration; `connector certify` checks observed processing evidence.
Certification requires a real canary and matching known builds, not merely a
successful setup or notification subprocess exit.

Extracted core baseline: `ca0bb9c1a33d090d9823950764223642096ab5e1`.
Plugin baseline: `6dc3cea798f14d686961b5bdffa106d16b9f7f96`.
Record tested binary versions and hashes with releases. Compatibility with other
core artifacts must be verified using the binary smoke check and relevant canary.

## Wake adapter subprocess protocol v1

A supporting core launches the operator-selected absolute connector executable
with the single argument `notify`. Stdin is exactly one JSON request, at most
64 KiB. It includes `version: 1`, `harness`, `session_id`, `delivery_handle`,
`message_id`, `schema_version`, and optionally `wake_id`, `digest_thread_count`,
and `codex_binary`. See `notify.Request` for the authoritative schema.

There is no message body, sender prose, recipient selection, or authorization
input. A delivery handle can contain local endpoint credentials: never log the
request. The daemon must authorize the delivery before invoking the adapter.

Stdout is one JSON result with `version: 1`, `accepted`, and `detail`. The daemon
bounds response size, applies a timeout, and treats execution failures, malformed
responses, or rejected attempts as failures. It does not fall back to a different
adapter after failure. An accepted wake is not proof of processing: claim/ack
remain separate daemon operations. The private daemon persists only bounded
fixed outcome labels, not arbitrary adapter stderr or detail.

## Source boundary

Public: lifecycle hydration, MCP, daemon client, install/configuration/diagnostics,
terminal monitor, file-descriptor liveness, wake adapters, manifests, and skills.

Private: daemon listener, authorization, identity ownership decisions, delivery
scheduling, SQLite, desktop/gateway, and managed agent lifecycle. No server or
storage implementation is copied into this repository. `scripts/check-boundary.py`
checks dependencies and source imports in CI.

The private core currently retains its existing implementation for backward
compatibility. The new public executable is selected explicitly; this extraction
does not silently change installed agents. Core's existing vendor workflow can
continue pinning connector package assets. Do not edit that vendored copy directly.

## Managed agent limitation

The private startup hook has an additional `recordAgentSession` side effect that
writes managed-agent resume receipts through its internal agent runtime. The
standalone public hook intentionally does not access that private state. Terminal
connectivity works without it. Do not replace the hook for managed desktop agents
that rely on automatic resume until core exposes a supported receipt endpoint.
This repository does not implement desktop commands, service supervision, or
managed agent start/stop/resume. Experimental host injection is not a supported
public runtime mode; use the ordinary Claude hook-long-poll mode.

## Terminal session verification

`cmd/holler-connector/session_test.go` uses a framed-JSON socket fixture to verify
startup/resume/clear/compact hydration and exact-session expiry for Claude, Codex,
and OpenCode, without claiming or acknowledging inbox messages during hydration.
It also exercises Claude monitor reconnection, expired-registration recovery,
duplicate-monitor exclusion, Stop-continuation wake suppression, reference-only
notices, and terminal outcomes for ended/superseded/adopted identities. Result-pipe
closure is covered separately. Codex attention tests cover PostToolUse/Stop output,
quiet failure, and suppression of child-agent and repeated Stop hooks. OpenCode
plugin tests cover registration, context injection, compaction, and unregistration.

The extracted monitor was corrected to stop on stale/adopted identity errors
returned during attach or wait, rather than retrying them indefinitely. These
checks exercise client behavior; they do not replace live harness certification.
Managed-agent resume receipts are a separate deferred feature and do not block
the terminal-session connector scope.
