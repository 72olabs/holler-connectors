# Holler Connectors

Holler lets coding agents in separate terminals message each other and the person running them. A local daemon, `hollerd`, stores and routes the messages. It is a separate, closed-source program and is not in this repository.

This repository is the agent side: the `holler-connector` binary and the plugins that attach it to Claude Code, Codex and OpenCode. The binary has four jobs:

- **Lifecycle hooks.** Register the session with the daemon, tell the agent what is unread, mark the session ended.
- **MCP server.** The tools an agent uses to send, claim and acknowledge messages.
- **Wakes.** The nudge that tells an idle agent a message arrived.
- **Setup.** `connector setup`, `doctor` and `launch` install and diagnose the plugins.

It never starts a daemon or opens the daemon's database. This repository is public: keep private paths, tokens, transcripts and delivery handles out of code, fixtures, commits and PR text.

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

1. **The live daemon and its state.** `~/.holler/` holds the developer's socket (`holler.sock`), connector settings (`connectors/*.json`) and `bin-path`, the file every installed plugin reads to find its binary. Commands here default to that socket. Do not send to it, register with it, write under `~/.holler`, or start or stop `hollerd`. To exercise the binary, point `--socket` or `HOLLER_SOCKET` at a socket you serve from a temporary directory, as the fixture in `cmd/holler-connector/session_test.go` does.
2. **The real harness configuration.** `connector setup --apply` runs the real `claude` or `codex` (`plugin marketplace add`, `plugin install`) and writes `~/.claude/settings.json`, `~/.codex/<profile>.config.toml`, `~/.holler/connectors/<harness>.json` and `~/.holler/bin-path`. `connector launch` starts the real harness. Run neither against the real home. Without `--apply`, setup prints its plan and writes nothing. With a temporary `HOME` and the variables in rule 3 unset, every path setup chooses by default is under that `HOME`.
3. **Your own session's variables.** A Holler-connected session exports `HOLLER_*`, and the harness exports `CLAUDE*` or `CODEX*`. The code under test reads them: `HOLLER_SOCKET`, `HOLLER_ACTOR`, `HOLLER_LAUNCH_TAG`, `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `CLAUDE_CODE_ENTRYPOINT` and others. `./scripts/check.sh` unsets all three families first. A bare `go test` does not: with `HOLLER_LAUNCH_TAG` set, four tests in `cmd/holler-connector` fail. Use the command under [Verifying](#verifying).

## Hit every seam

A change that is correct in this checkout is often half a change. Before calling work done, walk this list and say which entries applied.

### Three plugins, and Claude Code and Codex behave differently

Each harness has its own `hooks/hooks.json`, `.mcp.json`, `scripts/holler` wrapper and a hand-maintained copy of `skills/holler/SKILL.md` (about 24 KB each; `diff` the copies before and after an edit). A change to one needs a decision for the others, even if the decision is "not here".

| | Claude Code | Codex |
|---|---|---|
| Wake | `monitor`, an `asyncRewake` hook started on `SessionStart`, `Stop` and `StopFailure`. It runs in the hook process, long-polls the daemon, and exits 2 with one line on stderr. | The daemon runs `codex queue --thread <id> --message <notice>`. This binary takes part only if the daemon was started with `--connector-binary`, through `notify`. |
| Other wake hooks | none | `codex-attention` on `PostToolUse` (adds context) and on `Stop` (returns `decision: block`) |
| Hook syntax | `command` plus `args`, `${CLAUDE_PLUGIN_ROOT}` | one shell string, `${PLUGIN_ROOT}`, `additionalContextLimit: 1000` |
| Permissions setup writes | `mcp__plugin_holler_holler__<tool>` in the allow and ask lists of `settings.json` | a profile TOML with `enabled_tools` and an `approval_mode` per tool |
| Wrapper finds no binary | exits 0 for every command | exits 0 for hook commands, 2 for `mcp` |
| One-shot runs | wrapper skips `monitor` when `CLAUDE_CODE_ENTRYPOINT` is `sdk-cli`, `sdk-ts` or `sdk-py`, so there is no live wake | no special case |

Behaviour seen with real harnesses (Claude Code 2.1, Codex 0.166) that no fixture here reproduces:

- Claude Code runs `SessionStart` at launch. Codex runs it when the first prompt is submitted, so an untouched Codex window is not registered and cannot be woken.
- Codex asks the user to review hooks again whenever they change.
- Codex fires `SessionEnd` when it restarts its app-server (for example to update itself) with the terminal still open. The session stays ended until the next prompt.
- Codex's app-server outlives its terminal, so a wake can run a turn nobody sees. Claude's monitor stops when Claude closes its result pipe (`internal/fdliveness`).
- In `allocate` name mode a fresh `claude` in the same directory is a new actor. `claude --resume` keeps the actor, because the session id is a continuity handle (`RuntimeBinding.ContinuityHandles`).

If you touch registration, session end, identity or wake, reason through these cases and say in the PR which you could not test.

### What changes together

- **An MCP tool's name, schema or description** (`internal/mcp`) changes `ToolSurfaceHash()`. Update `commonTools` in `internal/connector/manifest.go`, and the `tools` list and `tool_surface_hash` in all three `connector.json` files (`holler-connector connector manifest --harness claude` prints the new hash). Then search for the tool's name in the text agents are shown: the three skills, `bus/conversation.go`, the notices in `cmd/holler-connector/main.go` and `codex_attention.go`, `codexqueue/adapter.go`, `opencodeprompt/adapter.go`, and `runtime.go` and `routing.go` in `internal/connector`. Users get permission entries for a new tool only when they rerun setup.
- **Any file in a plugin's `required_assets`** (hooks, wrapper, `.mcp.json`, `plugin.json`, skills) changes that plugin's hash. Update `package_hash` in its `connector.json` and the matching `PackageHash` in `manifest.go`; the failing `TestConnectorPackagesMatchFrozenManifests` prints the new value. A Codex hook change must also satisfy the event set pinned in `scripts/validate.py`.
- **The version** lives in `ConnectorVersion` (`manifest.go`), `connector_version` in three `connector.json` files, `version` in both `plugin.json` files and in `.claude-plugin/marketplace.json`.
- **A wire type or operation** (`bus/`, `api/`). The daemon decodes requests strictly, so a new request field makes an older daemon reject the whole call. Send it only when `hello` advertised a capability for it, as `RegisterSession` does for `runtime.host_app`, and add a test beside `api/compat_test.go` that the old path still works. `notify.Request` is decoded strictly in the other direction, daemon to this binary.
- **A connector settings field** (`~/.holler/connectors/<harness>.json`). `Load*ConnectorConfig` rejects unknown fields, and more than one binary reads that file (see the next section), so a new field breaks whichever reader is older.

### The second copy

The core daemon's repository carries its own copy of most of this code: `bus`, the `api` client, `internal/mcp`, `internal/connector`, `codexqueue`, `opencodeprompt`, `internal/fdliveness`, `internal/untrusted`, `buildinfo`, and the `hook`, `monitor`, `session-end`, `codex-attention` and `mcp` commands. Only `notify/`, and `notify.go` and `setup_defaults.go` in `cmd/holler-connector`, exist here alone. Both copies ship, and which one runs a user's hooks depends on how Holler was installed. They have already drifted: `ManagedWakeText` in `bus/conversation.go` differs between them at the time of writing.

You cannot see or edit the other copy from here. When you change a mirrored file, list the files under Compatibility in the PR description so a maintainer can port the change. A fix that lands in one copy is fixed for some users only.

### How a change reaches users

Merging here ships nothing by itself.

1. The core vendors `connectors/marketplace/` at a pinned commit of this repository, with a hash per file, and keeps the same `PackageHash` constants in its copy of `manifest.go`. Do not edit that vendored copy; change it here.
2. Applications that bundle the connector pin a commit of this repository.
3. Both pins are moved by hand. PRs here are squash-merged, so a branch commit's SHA does not exist on `main`: pin the commit on `main` after the merge.
4. On a user's machine the harness keeps its own cached copy of the plugin directory, which changes when setup is rerun. The Claude and Codex wrappers resolve the binary on every invocation (`HOLLER_BIN`, then `~/.holler/bin-path`, then `holler` on `PATH`). An MCP server or `monitor` that is already running keeps the old binary until the session restarts. So new code must work with an older plugin, and a new plugin with an older daemon.

## Where code lives

- `cmd/holler-connector/`: the CLI. `main.go` dispatches commands and holds `monitor` (the Claude wake loop), `hook` (SessionStart), `session-end`, `mcp` and `connector setup|launch|doctor|certify|manifest`. `codex_attention.go` is the Codex mid-turn hook. `agent_session.go` hands a session receipt to a core that launched the agent. `notify.go` is the wake-adapter entry point.
- `api/`: client for the daemon's Unix socket: length-prefixed JSON frames, the `hello` negotiation, capability names, and the mapping between wire error codes and `bus.Err*` values.
- `bus/`: wire types and validation shared with the daemon, and the managed wake text.
- `internal/mcp/`: the MCP stdio server: tool definitions (`server.go`, `channel_tools.go`, `attention_tools.go`), `ToolSurfaceHash`, and the untrusted message envelope.
- `internal/connector/`: setup and removal (the code that writes harness configuration), launch, doctor, certify, identity resolution (`binding.go`), SessionStart text (`runtime.go`, `routing.go`) and the frozen manifest (`manifest.go`).
- `notify/`, `codexqueue/`, `opencodeprompt/`: wake adapters: the subprocess protocol the daemon calls, and delivery into Codex and OpenCode.
- `internal/fdliveness/`: notices that the harness closed the monitor's pipe. `internal/untrusted/`: makes peer-controlled text safe to place in hook output.
- `connectors/marketplace/`: the three plugins and the two marketplace manifests.
- `scripts/`: `check.sh`, `validate.py` (plugin structure and hashes), `check-boundary.py` (no private or database dependency), `package.py` (local archive), `smoke-core.py` (needs a `hollerd` binary). `tests/test_packages.py` tests the plugins and wrappers.
- `docs/`: `COMPATIBILITY.md` describes the wire and adapter protocols. `launch-verification.md` is a historical record, not instructions. Where a doc and the code disagree, the code is right.

## Verifying

Run everything in a clean environment. A temporary `HOME` keeps tests away from real configuration, and the two cache variables stop Go from downloading and rebuilding everything under it:

```sh
env -i HOME="$(mktemp -d)" PATH="$PATH" GOCACHE="$(go env GOCACHE)" GOMODCACHE="$(go env GOMODCACHE)" \
  ./scripts/check.sh
```

`check.sh` runs `validate.py`, the Python tests, `check-boundary.py`, `go test ./...`, `go vet ./...` and builds `dist/holler-connector`. It needs Go 1.26+, Python 3 and Node (CI uses 22; the OpenCode plugin test fails without it). Use the same `env -i …` prefix for anything narrower, such as `go test ./internal/mcp`.

Measured on an Apple Silicon laptop with a warm build cache:

| Command | Time |
|---|---|
| `./scripts/check.sh` | 1.3 to 2.9 s |
| `go build ./...` | 0.6 s |
| `go vet ./...` | 0.3 s |
| `go test -count=1 ./...` | 1.3 to 1.8 s |
| `go test -race -count=1 ./...` | 3.3 s |
| `python3 scripts/package.py --version ci` | 1.5 s |

CI (`.github/workflows/ci.yml`) runs `validate.py` and the Python tests in the `packages` job, which is a required check, and `check.sh`, `go test -race ./...` and `package.py --version ci` on Linux and macOS in the `runtime` job. All of it takes under ten seconds locally, so run all of it.

The suite uses fake harnesses and socket fixtures. It does not start a real harness or a real daemon, so it cannot show that a hook fires, that a wake arrives, or that a harness accepts a changed plugin. Say in the PR what was covered by fixtures and what would need a live run.

## Invariants

Each of these holds in the code today and is checked by the suite. Keep them true, and extend the check when you extend the behaviour.

1. **An absent or failing daemon never blocks the harness.** With no daemon, `hook` exits 0 and its JSON says the connector is DEGRADED; `session-end` exits 0 with `{"ok": false}`; `codex-attention` exits 0 with `{}` on any failure. The Claude and Codex wrappers exit 0 for hook commands when no binary is found. `mcp` is the exception: with no daemon it exits non-zero instead of pretending to connect. Tests: `TestMissingDaemonDoesNotBlockLifecycle`, `TestCodexAttentionInvalidOrContinuationInputIsQuiet`, `tests/test_packages.py`.
2. **A wake carries a reference, never content.** Wake text holds a message or wake id, at most a count, and fixed instructions. It never holds a body, sender, title or channel name. This covers `monitor` stderr, `codex-attention` output, `notify.Request` and `bus.ManagedWakeText`. A harness accepting a wake does not mean the message was processed; only claim followed by ack does. Tests: `TestMonitorReconnectsAndEmitsReferenceOnlyWake`, `bus/managed_wake_test.go`, `notify/notify_test.go`.
3. **Hooks read; only the agent consumes.** `hook` lists what is unread and `codex-attention` polls for a notice; neither claims or acks. Claim and ack happen through MCP tools.
4. **Text from other agents is data, not instructions.** `internal/mcp/untrusted.go` marks every message object a tool returns with `trust: "untrusted"` and never interprets `body`. Peer-controlled strings placed in hook text (aliases, channel titles, condition summaries) pass through `untrusted.HookText`, which removes control characters and escapes `[`, `]`, `<` and `>` so a name cannot imitate Holler's own framing. New agent-visible text that includes a peer-controlled field must do the same. Tests: `internal/mcp/untrusted_test.go`, `internal/untrusted/text_test.go`.
5. **The connection fixes who the caller is.** The daemon binds actor and run at `hello`, and no tool takes an argument that names the sender or the acting identity. Tests: `TestManagedConsumeToolsHaveFixedLanesAndNoPrincipalSelectors`, `TestAttentionToolsHaveFixedSelfOnlyWriteLanes`.
6. **Setup previews, then writes.** Without `--apply`, setup returns its plan and changes nothing. With it, Claude and Codex configuration is written by `writeConfigBytes`: temporary file then rename, existing mode kept, symlinks followed, the first `.bak` never overwritten, and settings Holler does not own left in place. Tests: `TestClaudeSetupDryRunMakesNoChanges`, `TestConfigWritesPreserveSymlinkModeAndFirstBackup`.
7. **No private or storage dependency.** `scripts/check-boundary.py` fails on an import of the core module, a SQLite driver, or a directory such as `internal/store`. Do not add one, or a `replace` directive, to make a test pass.

## Traps

The three that have cost the most are described above: the second copy, hashes and pins that are moved by hand, and session variables leaking into tests. The rest:

- **Exit codes and output streams are the hook contract.** Claude Code shows the model whatever `monitor` wrote to stderr when it exits 2, so that line is a prompt: do not log to stderr on the wake path. The harness reads the stdout of `hook`, `session-end` and `codex-attention` as JSON: print nothing else there.
- **`session-end` is not fully fail-open.** Malformed hook input makes it exit 1 or 2 today. Do not copy that into a new hook; fail quietly as `codex-attention` does.
- **Two generations of tools share nouns.** `bus_inbox` claims every available legacy message, `bus_check_inbox` only lists them, and `holler_channel_inbox` only lists managed deliveries. `bus_ack` does not close a managed delivery. When hook or skill text tells an agent what to call, name a real tool from the right generation. The SessionStart text says "use channel.list", which is a capability reached through `holler_read`, not a tool; do not repeat that pattern.
- **Agent-visible text is paid for in every session.** SessionStart context is about 800 bytes with nothing unread, the channel directory can grow it to 16 KB, and Codex hooks declare a context limit of 1000. Each skill is about 24 KB. Before adding a sentence, check whether the tool can enforce the rule instead.
- **Messages say `holler connector …`.** Setup plans, errors and the DEGRADED hook text were written for the core's CLI, where the command is `holler`. In this binary the same command is `holler-connector connector …`.
