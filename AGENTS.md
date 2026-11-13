# Holler Connectors

Holler lets coding agents in separate terminals message each other and the person running them. A local daemon, `hollerd`, stores and routes the messages. It is a separate, closed-source program and is not in this repository.

This repository is the agent side: the `holler-connector` binary and the plugins that attach it to Claude Code, Codex and OpenCode. The binary has four jobs:

- **Lifecycle hooks.** Register the session with the daemon, tell the agent what is unread, mark the session ended.
- **MCP server.** The tools an agent uses to send, claim and acknowledge messages.
- **Wakes.** The nudge that tells an idle agent a message arrived.
- **Setup.** `connector setup`, `doctor` and `launch` install and diagnose the plugins.

It never starts a daemon or opens the daemon's database. This repository is public: keep private paths, tokens, transcripts and delivery handles out of code, fixtures, commits and PR text.

The public repository is generated from a directory of a private source repository, where this code is edited (see `CONTRIBUTING.md`). This file is published with it, so it describes what a reader of the public copy can see. If your checkout has this directory inside a larger repository, read that repository's `AGENTS.md` as well: it says where the other side is, and its rules come first.

## Words you will meet

- **harness**: the agent CLI, meaning Claude Code (`claude`), Codex (`codex`) or OpenCode (`opencode`). Manifests call it the client (`client_command`). `api.Client` is something else: the client of the daemon.
- **daemon**, **core**, **engine**, **hollerd**: one thing. **bus** is its older name and survives in the `bus/` package and the `bus_*` tools.
- **actor**: a durable identity and address, such as `claude-82a8af`. An **alias** is a name a person chose that points at an actor. A **run** is one harness process (`HOLLER_RUN`). A **session** is one harness conversation, named by the `session_id` in hook input. A **registration** is the daemon's five-minute lease saying a session is live; the MCP server renews it.
- **wake**, **attention**, **notification**, **notice**: one thing. The **attention mode** is how a session receives it: `hook-long-poll` (Claude), `native-queue` (Codex), `native-prompt` (OpenCode) or `startup-only` (no live wake).
- **channel**, **conversation**: one thing. A **room** is a named channel. **Managed** means channel messages (the `holler_channel_*` tools). **Legacy** means direct actor-to-actor messages (the `bus_*` tools). Both are in use.
- **delivery**: a message owed to an actor. The agent **claims** it under a timed **lease**, then **acks** it, or **nacks** it to give it back.
- **capability** has two meanings: a feature string the daemon advertises in `hello` (`channel-room-access-v1`), and a named daemon operation (`channel.get`) reached through the `holler_read` and `holler_write` tools. Neither is an MCP tool name.
- **plugin**, **package**: one directory under `connectors/marketplace/plugins/`. `claude-holler` is Claude's and `opencode-holler` is OpenCode's. Plain `holler` is Codex's; it is not shared.

## Ways to hurt the machine you run on

The developer whose machine this is probably uses Holler, and you may be running inside a session it connected.

1. **The live daemon and its state.** `~/.holler/` holds the developer's socket (`holler.sock`), connector settings (`connectors/*.json`) and `bin-path`, the file every installed plugin reads to find its binary. Commands here default to that socket. Do not send to it, register with it, write under `~/.holler`, or start or stop `hollerd`. To exercise the binary, point `--socket` or `HOLLER_SOCKET` at a socket you serve from a temporary directory, as the fixture in `cli/session_test.go` does.
2. **The real harness configuration.** `connector setup --apply` invokes the real `claude` or `codex` program (`plugin marketplace add`, `plugin install`, `plugin add`), and that program writes wherever it chooses. Setup then writes its own files: Claude settings (`CLAUDE_CONFIG_DIR` or `~/.claude`), the Codex profile (`CODEX_HOME` or `~/.codex`), the OpenCode package (`XDG_CONFIG_HOME` or `~/.config`, under `opencode/holler`), `~/.holler/connectors/<harness>.json` and, for Claude and Codex, `~/.holler/bin-path`. `connector launch` starts the real harness. Run neither against the real home. Without `--apply`, setup prints its plan and writes nothing. Under `env -i` with a temporary `HOME`, those write destinations move under it. That does not contain what the harness program does, and setup still records paths outside it: the running binary, and the working directory as project root.
3. **Your own session's variables.** A Holler-connected session exports `HOLLER_*`, and the harness exports `CLAUDE*` or `CODEX*`. The code under test reads them: `HOLLER_SOCKET`, `HOLLER_ACTOR`, `HOLLER_LAUNCH_TAG`, `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `CLAUDE_CODE_ENTRYPOINT` and others. `./scripts/check.sh` unsets all three families first. A bare `go test` does not: with `HOLLER_LAUNCH_TAG` set, five tests in `cli` fail. Use the command under [Verifying](#verifying).

## Hit every seam

A change that is correct in this checkout is often half a change. Before calling work done, walk this list and say which entries applied.

### Three plugins, three shapes

Each plugin has its own `scripts/holler` wrapper and a hand-maintained copy of `skills/holler/SKILL.md` (22 to 24 KB; `diff` the copies before and after an edit). A change to one needs a decision for the others, even if the decision is "not here". Claude Code and Codex are both driven by `hooks/hooks.json` and `.mcp.json`, and differ like this:

| | Claude Code | Codex |
|---|---|---|
| Wake | `monitor`, an `asyncRewake` hook started on `SessionStart`, `Stop` and `StopFailure`. It runs in the hook process, long-polls the daemon, and exits 2 with one line on stderr. | The daemon runs `codex queue --thread <id> --message <notice>`. This binary takes part only if the daemon was started with `--connector-binary`, through `notify`. |
| Other wake hooks | none | `codex-attention` on `PostToolUse` (adds context) and on `Stop` (returns `decision: block`) |
| Hook syntax | `command` plus `args`, `${CLAUDE_PLUGIN_ROOT}` | one shell string, `${PLUGIN_ROOT}`, `additionalContextLimit: 1000` |
| Permissions setup writes | `mcp__plugin_holler_holler__<tool>` in the allow and ask lists of `settings.json` | a profile TOML with `enabled_tools` and an `approval_mode` per tool |
| Wrapper finds no binary | exits 0 for every command | exits 0 for hook commands, 2 for `mcp` |
| One-shot runs | wrapper skips `monitor` when `CLAUDE_CODE_ENTRYPOINT` is `sdk-cli`, `sdk-ts` or `sdk-py`, so there is no live wake | no special case |

OpenCode has neither file. Its package is `plugins/holler.js`, the wrapper and two skills (see `required_assets` in its `connector.json`):

- `plugins/holler.js` runs inside OpenCode and is the lifecycle. It spawns the binary itself (`HOLLER_BIN`, else `holler` on `PATH`) with `hook` or `session-end`, treats a non-zero exit as failure, parses `hook` stdout as JSON, and injects the context through OpenCode's system-prompt and compaction hooks. On failure it logs a warning and injects its own DEGRADED text.
- Setup generates the MCP entry and the permissions (`holler_<tool>`: `allow` or `ask`) into `opencode.json` in the installed package (`openCodeProfile`). No such file is shipped.
- The wake is an HTTP POST to OpenCode's loopback server (`opencodeprompt`). The server address and its credentials travel in the delivery handle, which is why a handle is never logged.
- Its wrapper tries only `HOLLER_BIN`, then `holler` on `PATH`, then exits 2. It does not read `bin-path`.

Observed on 2026-11-12 in live sessions with Claude Code 2.1.322 and Codex 0.166.0. No fixture here reproduces these, and other harness versions may differ:

- Claude Code ran `SessionStart` at launch. Codex ran it when the first prompt was submitted, so an untouched Codex window was not registered and could not be woken.
- Codex showed a hook-review prompt at launch because the plugin's hooks were new or changed.
- Codex fired `SessionEnd` when it restarted its app-server to update itself, with the terminal still open. The session stayed ended until the next prompt.
- Codex's app-server outlived its terminal, so a wake ran a turn nobody saw. Claude's monitor stops when Claude closes its result pipe (`fdliveness`).
- In `allocate` name mode a fresh `claude` in the same directory was a new actor. `claude --resume` kept the actor; the session id is a continuity handle (`RuntimeBinding.ContinuityHandles`).

If you touch registration, session end, identity or wake, reason through these cases and say in the PR which you could not test.

### What changes together

- **An MCP tool's name, schema or description** (`mcp/`) changes `ToolSurfaceHash()`. Update `commonTools` in `connector/manifest.go`, and the `tools` list and `tool_surface_hash` in all three `connector.json` files (`holler-connector connector manifest --harness claude` prints the new hash). Then search for the tool's name in the text agents are shown: the three skills, `bus/conversation.go`, the notices in `cli/cli.go` and `cli/codex_attention.go`, `codexqueue/adapter.go`, `opencodeprompt/adapter.go`, and `runtime.go` and `startup.go` in `connector/`. Users get permission entries for a new tool only when they rerun setup.
- **Any file in a plugin's `required_assets`** (hooks, wrapper, `.mcp.json`, `plugin.json`, `plugins/holler.js`, skills) changes that plugin's hash. Update `package_hash` in its `connector.json` and the matching `PackageHash` in `manifest.go`; the failing `TestConnectorPackagesMatchFrozenManifests` prints the new value. A Codex hook change must also satisfy the event set pinned in `scripts/validate.py`.
- **What `hook` or `session-end` reads, prints or returns** has three callers: Claude's `hooks.json`, Codex's `hooks.json`, and `plugins/holler.js`. Check the JavaScript too; its tests are in `connector/opencode_plugin_test.go` and need Node.
- **The SessionStart text** (`connector/startup.go`, `connector/runtime.go`) is pinned byte for byte, with the daemon calls the hook makes, by `TestStartupTextMatchesGoldens` (`cli/startup_golden_test.go`, goldens in `cli/testdata/startup/`). The wording was chosen by trials with real agents. An intended change regenerates the goldens with `go test ./cli -run TestStartupTextMatchesGoldens -startup-golden-update`; review the diff, then say under Compatibility in the PR what agents will read differently.
- **A command, its flags, output or exit code** (`cli/`). `cli` is public API and the command layer of every binary built on it, the daemon's own CLI included: it runs `cli.Run` with its own `Options` (usage text, extra commands, dial options, session recorder). A command every such binary should have goes in `cli`; a command only one binary has is registered by that binary through `Options.Commands`, and `cli` never names it. Its exported names (`Run`, `Main`, `Options`, `Command`, `ExitCode`, `SessionRecorder`, `Notify`, `DefaultSocketPath`) change only with the daemon's side in step. Hook and `monitor` output is a prompt (see Traps), and JSON output is parsed by other programs.
- **The version** lives in `ConnectorVersion` (`manifest.go`), `connector_version` in three `connector.json` files, `version` in both `plugin.json` files and in `.claude-plugin/marketplace.json`.
- **A wire type or operation** (`bus/`, `api/`). The daemon decodes requests strictly, so a new request field makes an older daemon reject the whole call. Send it only when `hello` advertised a capability for it, as `RegisterSession` does for `runtime.host_app`, and add a test beside `api/compat_test.go` that the old path still works. `notify.Request` is decoded strictly in the other direction, daemon to this binary.
- **A connector settings field** (`~/.holler/connectors/<harness>.json`). `Load*ConnectorConfig` rejects unknown fields, so a file written by a newer binary cannot be read by an older one.

### The daemon on the other side

The daemon speaks the wire types in `bus/` and `api/`. It builds against this module and imports twelve of its packages: `api`, `bus`, `buildinfo`, `cli`, `codexqueue`, `connector`, `notify`, `opencodeprompt`, `untrusted`, `unread`, `fdliveness` and `mcp`. A change to them, including to their exported Go API, is also a change to the daemon, and the daemon's own checks run on it before it merges. `api.Client` is the daemon's socket client too: it adds its own operations through `Call` and `WithErrorDecoder`, and its server reads and writes frames with `ReadFrame` and `WriteFrame`. `connector` is used by the daemon through its exported names only; the daemon delivers wakes with its own code, through `notify`, `codexqueue` and `opencodeprompt`. The daemon's own CLI is a second `main` over `cli`: it runs every command here, and adds commands of its own through `Options.Commands`. From the public copy you cannot see or test the daemon's source; `scripts/smoke-core.py` checks a daemon binary if you have one. When a change touches a wire type, an operation, an error code, wake text, hook behaviour or a plugin file, say so under Compatibility in the PR description so the matching change can be made on the other side.

### What a user's machine holds

Merging here ships nothing by itself, and an installed machine holds pieces of different ages:

- Plugin files are installed through the harness (`claude plugin install` or `update`, `codex plugin add`) or copied by OpenCode setup, so users get changed hooks, wrappers and skills when setup is rerun.
- The Claude and Codex wrappers resolve the binary on every invocation (`HOLLER_BIN`, then `~/.holler/bin-path`, then `holler` on `PATH`), so a replaced binary takes effect at the next hook.
- An MCP server or `monitor` that is already running keeps the old binary until the session restarts.

So new code must work with an older plugin, and a new plugin with an older daemon. Commits on the public `main` are syncs from the private source (before that, squash merges), so a public commit hash says which published tree you have, not which change made it.

## Where code lives

- `cli/`: the command layer. `cli.go` holds `Run` (the dispatch), `monitor` (the Claude wake loop), `hook` (SessionStart), `session-end`, `mcp`, `connector setup|launch|doctor|certify|manifest` and the messaging commands. `codex_attention.go` is the Codex mid-turn hook. `agent_session.go` is the default session recorder: it hands a session receipt to whatever launched the agent. `notify.go` is `Notify`, the wake-adapter entry point. `setup_defaults.go` keeps a reconnecting `connector setup` on the saved identity and attention mode.
- `cmd/holler-connector/`: the binary, a thin `main` over `cli`: its usage text and the `notify` command.
- `api/`: client for the daemon's Unix socket: length-prefixed JSON frames (`WriteFrame`, `ReadFrame`), the `hello` negotiation, capability names, and the mapping from wire error codes to `bus.Err*` values (`RPCError.Err`). The daemon assigns the codes; a code this client does not know becomes a plain error with the daemon's message.
- `bus/`: wire types and validation shared with the daemon, and the managed wake text.
- `mcp/`: the MCP stdio server: tool definitions (`server.go`, `channel_tools.go`, `attention_tools.go`), `ToolSurfaceHash`, and the untrusted message envelope.
- `connector/`: setup and removal (the code that writes harness configuration), launch, doctor, certify, identity resolution (`binding.go`), SessionStart text (`runtime.go`, `startup.go`) and the frozen manifest (`manifest.go`).
- `notify/`, `codexqueue/`, `opencodeprompt/`: wake adapters: the subprocess protocol the daemon calls, and delivery into Codex and OpenCode.
- `fdliveness/`: notices that the harness closed the monitor's pipe. `untrusted/`: makes peer-controlled text safe to place in hook output.
- `connectors/marketplace/`: the three plugins and the two marketplace manifests.
- `scripts/`: `check.sh`, `validate.py` (plugin structure and hashes), `check-boundary.py` (see invariant 7 for what it checks), `package.py` (local archive), `smoke-core.py` (needs a `hollerd` binary). `tests/test_packages.py` tests the plugins and wrappers.
- `docs/`: `COMPATIBILITY.md` describes the wire and adapter protocols. `launch-verification.md` is a historical record, not instructions. Where a doc and the code disagree, the code is right.

## Verifying

Run everything in a clean environment. A temporary `HOME` keeps tests away from real configuration, and the two cache variables stop Go from downloading and rebuilding everything under it:

```sh
env -i HOME="$(mktemp -d)" PATH="$PATH" GOCACHE="$(go env GOCACHE)" GOMODCACHE="$(go env GOMODCACHE)" \
  ./scripts/check.sh
```

`check.sh` runs `validate.py`, the Python tests, `check-boundary.py`, `go test ./...`, `go vet ./...` and builds `dist/holler-connector`. It needs Go 1.26+, Python 3 and Node (CI uses 22; the OpenCode plugin test fails without it). Use the same `env -i …` prefix for anything narrower, such as `go test ./mcp`.

CI (`.github/workflows/ci.yml`) runs `validate.py` and the Python tests in the `packages` job, which is a required check, and `check.sh`, `go test -race ./...` and `python3 scripts/package.py --version ci` on Linux and macOS in the `runtime` job.

For scale, measured on 2026-11-12 on an Apple Silicon laptop with a warm build cache: `check.sh` 1 to 3 s (of which `go build ./...` 0.6 s, `go vet ./...` 0.3 s, `go test -count=1 ./...` under 2 s), the race run about 3 s, packaging about 2 s. Running everything CI runs is cheap; do it before you push.

The suite uses fake harnesses and socket fixtures. It does not start a real harness or a real daemon, so it cannot show that a hook fires, that a wake arrives, or that a harness accepts a changed plugin. Say in the PR what was covered by fixtures and what would need a live run.

## Invariants

Each of these holds in the code today and is checked by the suite. Keep them true, and extend the check when you extend the behaviour.

1. **A daemon that cannot be reached does not block the harness.** Tested: against a missing socket, `hook` exits 0 with JSON that says DEGRADED and `session-end` exits 0 with `{"ok": false}` (`TestMissingDaemonDoesNotBlockLifecycle`); `codex-attention` prints `{}` for invalid input (`TestCodexAttentionInvalidOrContinuationInputIsQuiet`) and its code does the same on every error path; with `HOLLER_BIN` unset and no binary found, the Claude and Codex wrappers exit 0 for hook commands (`tests/test_packages.py`, `claude_plugin_test.go`). Not covered: every wrapper runs `exec "$HOLLER_BIN"` without checking it, so a stale override, including the `binary` option setup records for the Claude plugin, makes each hook fail instead of falling through. `mcp` fails on purpose when there is no daemon.
2. **A wake carries a reference, never content.** Wake text holds a message or wake id, at most a count, and fixed instructions. It never holds a body, sender, title or channel name. This covers `monitor` stderr, `codex-attention` output, `notify.Request` and `bus.ManagedWakeText`. A harness accepting a wake does not mean the message was processed; only claim followed by ack does. Tests: `TestMonitorReconnectsAndEmitsReferenceOnlyWake`, `bus/managed_wake_test.go`, `notify/notify_test.go`.
3. **Hooks do not claim or acknowledge deliveries.** SessionStart does write to the daemon: it registers the session, may claim the default alias, records hydration and may mark an operator condition about its own actor as presented. It lists unread messages with `CheckInbox` and the `channel.inbox` read capability (`unread`) and never calls claim or ack; `codex-attention` only polls for a notice. Deliveries are consumed through MCP tools. Tests: `TestSessionLifecycleRehydratesAndEndsExactSession` counts claim and ack calls, and `TestStartupCountsLegacyAndManagedUnread` fails on any claim, ack, nack, extend or write-capability call.
4. **Text from other agents is data, not instructions.** `mcp/untrusted.go` marks every message object a tool returns with `trust: "untrusted"` and never interprets `body`. Peer-controlled strings placed in hook text (aliases, channel titles, condition summaries) pass through `untrusted.HookText`, which removes control characters and escapes `[`, `]`, `<` and `>` so a name cannot imitate Holler's own framing. New agent-visible text that includes a peer-controlled field must do the same. Tests: `mcp/untrusted_test.go`, `untrusted/text_test.go`.
5. **The connection fixes who the caller is.** The daemon binds actor and run at `hello`, and no tool takes an argument that names the sender or the acting identity. Tests: `TestManagedConsumeToolsHaveFixedLanesAndNoPrincipalSelectors`, `TestAttentionToolsHaveFixedSelfOnlyWriteLanes`.
6. **Setup previews, then writes.** Without `--apply`, setup returns its plan and changes nothing. With it, Claude and Codex configuration is written by `WriteConfigBytes`: temporary file then rename, existing mode kept, symlinks followed, the first `.bak` never overwritten, and settings Holler does not own left in place. Tests: `TestClaudeSetupDryRunMakesNoChanges`, `TestConfigWritesPreserveSymlinkModeAndFirstBackup`.
7. **No dependency on the core or on a database.** The policy, from `CONTRIBUTING.md`: never add a private core module, a database driver or a local replacement dependency to make a test pass. `scripts/check-boundary.py` enforces part of it. It fails if `go list -m all` names the core module, `modernc.org/sqlite` or `github.com/mattn/go-sqlite3`, if a Go file imports the core module, or if `internal/store`, `internal/daemon`, `internal/gateway` or `internal/agentruntime` exists. It does not look for other drivers or for `replace` directives, so the rest of the policy depends on you.

## Traps

The costliest ones are described above: hashes that must move together, changes the daemon side never hears about, and session variables leaking into tests. The rest:

- **Exit codes and output streams are the hook contract.** Claude Code shows the model whatever `monitor` wrote to stderr when it exits 2, so that line is a prompt: do not log to stderr on the wake path. The harness reads the stdout of `hook`, `session-end` and `codex-attention` as JSON: print nothing else there.
- **`session-end` is not fully fail-open.** Malformed hook input makes it exit 1 or 2 today. Do not copy that into a new hook; fail quietly as `codex-attention` does.
- **Two generations of tools share nouns.** `bus_inbox` claims a bounded batch of available legacy messages (20 by default, at most 100; a larger `limit` is reset to 20), so a full inbox needs more than one call, although its description says "all". `bus_check_inbox` only lists them, and `holler_channel_inbox` only lists managed deliveries. `bus_ack` does not close a managed delivery. When hook or skill text tells an agent what to call, name a real tool from the right generation. A capability such as `channel.list` is reached through `holler_read`, not called as a tool: write "holler_read with capability channel.list" (`TestStartupNamesToolsNotCapabilities` checks SessionStart text for this).
- **Agent-visible text is paid for in every session.** SessionStart context is about 500 bytes with nothing unread, most of it the trust sentence (`startupTrustSentence`, a wording chosen by trials with real agents and pinned by a test), and is capped at 1000 bytes (`StartupContextBudget`, Codex's declared hook context limit); optional parts give way to a pointer at `bus_status` or `holler_read` when they do not fit, and the trust sentence never does. Each skill is 22 to 25 KB. Before adding a sentence, check whether the tool can enforce the rule instead.
- **Messages say `holler connector …`.** Setup plans, errors and the DEGRADED hook text name a `holler` command. The binary built here is `holler-connector`, so the command is `holler-connector connector …`.
