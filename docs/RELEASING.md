# Releases

This repository has no releases or tags of its own. It is a generated copy of a
directory in a private source repository (see `CONTRIBUTING.md`), and each
commit here is a sync of that directory.

## How the code reaches users

The connector ships inside Holler's own releases, built from the same source
commit as the tree published here:

- The Holler desktop app ships the `holler-connector` binary from
  `cmd/holler-connector` and the plugins in `connectors/marketplace`.
- The Holler command-line release ships `holler`, the daemon's CLI. It is built
  on package `cli`, so the shared built-in commands behave the same in it, and
  it ships the same plugin files under `share/holler/marketplace`.

Installed plugins find their binary on every run (`HOLLER_BIN`, then
`~/.holler/bin-path`, then `holler` on `PATH`), so either binary can serve them.

## Versions

The connector version is `ConnectorVersion` in `connector/manifest.go`, repeated
in each plugin's `connector.json`, both `plugin.json` files and
`.claude-plugin/marketplace.json`. It is also the version of the next Holler
release, whose packaging refuses plugin files whose versions do not match it.
Changing a plugin file changes its package hash (`AGENTS.md`, "What changes
together"); `holler-connector connector doctor` compares installed plugins with
those hashes.

## Before a change ships

1. `./scripts/check.sh` and `go test -race ./...` on macOS and Linux (CI runs
   both).
2. `python3 scripts/package.py --version VERSION` builds a local archive (the
   binary, the plugins, licences, notices and documentation) to test an install
   from. It is not published.
3. With a daemon binary, `scripts/smoke-core.py` checks the wire protocol against
   it.
4. A change to identity, permissions, hooks or wakes needs a live run with each
   supported harness version: start-up, resume and compaction, session end, a
   wake, and the startup-only fallback. The fixtures here cannot show that a
   harness fires a hook or accepts a changed plugin.
