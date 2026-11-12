---
name: holler-setup
description: Installs, configures, diagnoses, or changes the Codex Holler connector and its native-queue or startup-only attention mode. Use when a user asks to set up Holler, make Codex talkable, change wake behavior, or troubleshoot connector readiness.
---

## Standalone open-source runtime

When using `holler-connector` with a separately installed core binary, use
`holler-connector connector setup --harness HARNESS` to preview configuration,
then add `--apply` to install it. Launch with
`holler-connector connector launch --harness HARNESS`. Substitute this plugin's
harness name for HARNESS. Use `holler-connector connector doctor` for diagnostics.
The daemon must already be running; this executable does not install or supervise
core. Setup records its own executable for hook/MCP launches. Do not run the
private `holler setup` command afterward unless intentionally switching back to
that runtime. The integrated-core commands below apply to installations using
the private `holler` CLI, not to the standalone executable.


# Holler Setup

For ordinary first-time setup, use the opinionated product command:

```sh
holler setup codex
```

It defaults to allocated actor base `codex`, peer `claude`, project `default`, and live `native-queue` attention. It discovers the installed Holler marketplace, installs or refreshes the plugin, preserves and extends Codex user configuration with only the frozen Holler MCP allowlist, records absolute executable paths and connector identity, installs the per-user `hollerd` service, starts it, and verifies the socket. Existing connector identity—including legacy naming mode—wins on a rerun. The command previews exact paths and actions and asks for confirmation; use `--dry-run` for a non-mutating plan or `--yes` only when the user explicitly authorizes unattended application.

Codex owns executable-hook trust. On the first turn after installation or a package update, have the user inspect and trust the exact packaged `SessionStart`, `SessionEnd`, `PostToolUse`, and `Stop` commands and content hash when Codex prompts. Do not manipulate an internal trust store or recommend `--dangerously-bypass-hook-trust` for a normal session. Once trusted, normal `codex` launches reuse the connector automatically. Codex runs `SessionStart` on the first submitted turn, so native-queue addressability begins after that turn rather than when an untouched TUI opens.

This candidate requires Codex 0.166.0 or newer for its four reviewed hooks; compatibility is pending a fresh real-client certification. `PostToolUse` can add one reference-only inbox notice during a tool-using turn. `Stop` continues only for newly noticed addressed work, and never recursively continues a Stop-hook turn. Both paths preserve the normal MCP claim/ack boundary. Native queue notices already accepted by Codex cannot be cancelled: a residual empty turn remains possible after a hook drains that work. Hook duplicate suppression expires after five minutes; persistent attempt counters retain the finite shared notification budget. Lost hook output can retry while that budget remains. Exhaustion leaves work durable for explicit polling rather than generating unbounded model turns.

Rerun setup after upgrading Holler. Upgrading to 0.6.1 is the one-time bootstrap boundary for the fixed capability bridge: an MCP child that was already running 0.6.0 cannot gain those three tools, but sessions running 0.6.1 can discover later daemon capabilities without another tool-surface restart. Before uninstalling the package, use `holler setup codex --remove`; the command strips only the managed Codex policy and connector state and leaves the durable database intact.

Use advanced connector setup when the user requests a custom attention mode, identity, project, policy destination, marketplace, or preview/apply split. Do not edit Codex configuration from memory:

```sh
holler connector setup --harness codex --attention <native-queue|startup-only> \
  --actor <actor> --peer <peer> --project <project> --marketplace <path-or-source> --apply
```

Advanced setup defaults to preview-only and writes a dedicated `$CODEX_HOME/<profile>.config.toml`; apply only after the user authorizes the reported changes. The product command may merge its exact allowlist into general Codex config because the user invoked and confirmed product setup. It preserves unrelated TOML byte-for-byte, creates a backup before a change, is idempotent, and refuses a conflicting pre-existing Holler policy instead of broadening it.

Setup records absolute runtime and Codex executable paths so a background daemon or plugin process never depends on an interactive shell `PATH`. Once the plugin hooks are trusted and its MCP policy is admitted, starting plain `codex` loads the persisted actor, routing, socket, and attention selection. Plugin processes from one Codex process derive the same immutable run automatically.

Use the connector launcher when you want the dedicated least-privilege profile, an explicit actor override, or a separately addressable concurrent session:

```sh
holler connector launch --harness codex --actor <actor> -- [additional codex arguments]
```

The launcher supplies a fresh random run, the reviewed named profile, working tree, connector identity, and attention selection. Explicit launcher values always override persisted plain-session defaults.

New product installations use allocated naming so concurrent sessions receive opaque actor handles such as `codex-a7f3c2`; rerunning setup preserves an existing legacy selection. Use `--name-mode exact` when one durable actor must refuse a second live run and add launcher-only `--takeover` only for a deliberate supersession. Add `--launch-tag <stable-tag>` when a supervisor needs a restarted process to reclaim its previous allocation. Never generate takeover from an agent tool call.

Run the doctor command reported by setup when diagnosis or certification is requested. `CONFIGURED` proves static setup only. A `live-review` connector is `READY` only after a real post-registration native-queue wake is fetched, claimed, and acknowledged. `startup-only` can satisfy only `async-peer`.

Treat actor identity as an inbox identity, not a display name. In legacy mode concurrent sessions using one actor are competing consumers. Prefer allocate mode when each session must be independently addressable.

## Sessions run by a host

If another program started this session and told it that the program manages Holler attention, do not run setup or change attention mode for that session. The host overrides the setting, so the change would only alter the user's other sessions.
