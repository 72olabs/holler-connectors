# Holler Connectors

Open-source Claude Code, Codex, and OpenCode integrations for a separately
installed Holler core binary. Build and debug session hooks, MCP tools,
configuration, and wake adapters without access to private core source.

The connector is licensed under Apache-2.0. Holler core is distributed separately
under its own terms. This repository does not contain a daemon or database engine.

## Release status

This is a source preview for terminal sessions. A certified binary release and
a downloadable compatible Holler core artifact are release prerequisites, not
provided by this repository. Managed desktop-agent resume is not supported by
the standalone hooks yet; see [compatibility](docs/COMPATIBILITY.md).

## Build and test

```sh
git clone https://github.com/72olabs/holler-connectors.git
cd holler-connectors
```

Requirements: Go 1.26+, Python 3, Node.js 22+, and macOS or Linux. Unit tests use temporary
homes, fake harnesses, and socket fixtures; no vendor login or model calls.

```sh
./scripts/check.sh
./dist/holler-connector version
```

Build output is `dist/holler-connector`. Keep its absolute path stable after
installation. Existing plugin identities stay unchanged.

## Use with a core binary

Install the core binary separately. If its daemon is already running, set
`HOLLER_SOCKET` to that instance's socket. Otherwise start a dedicated instance:

```sh
mkdir -p "$HOME/.holler"
hollerd --db "$HOME/.holler/holler.db" --socket "$HOME/.holler/holler.sock"
export HOLLER_SOCKET="$HOME/.holler/holler.sock"
./dist/holler-connector status
```

Do not start a second daemon against an active instance's database/socket.

Preview connector setup, then apply the displayed changes:

```sh
./dist/holler-connector connector setup --harness claude
./dist/holler-connector connector setup --harness claude --apply
./dist/holler-connector connector launch --harness claude
```

Replace `claude` with `codex` or `opencode` as appropriate. Setup requires that
harness to be installed. It records the public executable in `~/.holler/bin-path`
and installs/configures the packaged plugins. The wrappers use that record even
when hooks have a minimal PATH. `HOLLER_BIN` can explicitly override the runtime.
Do not override it with the private CLI when testing the public runtime.

For manual configuration, point the plugin's `HOLLER_BIN` at the absolute path
to `holler-connector`. `go install` alone does not install plugin assets; use the
release archive layout or pass `--marketplace /absolute/path/to/connectors/marketplace`.
OpenCode also accepts `--package-source` for its plugin directory.

## Independently patch wake delivery

Claude's long-poll monitor runs in this public executable. Codex/OpenCode push
wakes originate in the daemon. A core build supporting the optional adapter
handoff must be started with:

```sh
hollerd --db "$HOME/.holler/holler.db" --socket "$HOME/.holler/holler.sock" \
  --connector-binary /absolute/path/to/holler-connector
```

The daemon still authorizes deliveries and owns retries. The public adapter
translates an authorized reference into a harness notification. Without this
flag, existing core binaries use their built-in push adapters: changing the
public adapter alone will not replace them. The flag is implemented in the
companion core change; do not assume an already installed release supports it.

## Verify a core artifact

```sh
python3 scripts/smoke-core.py --daemon /absolute/path/to/hollerd
```

Add `--external-wakes` to also require support for the adapter-handoff flag.

This starts the supplied binary in an isolated temporary home and checks the
wire handshake, send/inbox/claim/ack, and MCP initialize/tool discovery. It never
reads the database. Real harness wake/trust behavior still needs live validation.

See [compatibility and boundaries](docs/COMPATIBILITY.md),
[contributing](CONTRIBUTING.md), and [release steps](docs/RELEASING.md).

Report vulnerabilities privately to **security@72olabs.ai**; see [security policy](SECURITY.md).
