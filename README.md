# Holler connectors (private)

Source of truth for the installable Codex, Claude Code and OpenCode connector
packages. This is a private repository; it is not a new public marketplace.

## Boundary

`connectors/marketplace/` contains the existing plugin identities, hook wiring,
MCP launch configuration, shell launchers, skills and connector manifests.
These files were extracted byte-for-byte from `72olabs/holler` commit
`2f3bad0b27780b6f21180dcda7ae5c366ce8cc02` (0.8.0). No core source or source
repository history was copied. The inherited LICENSE remains unchanged.

Core runtime, setup policy, identity enforcement, queue adapters, doctor and
certification remain in the private `72olabs/holler` repository. This extraction
does not add support for a new harness or change installed users' configuration.

## Development and validation

```sh
python3 scripts/validate.py
python3 -m unittest discover -s tests -v
```

Package versions and hashes still need to match core's compiled manifests.
This is source ownership separation, not independent runtime compatibility yet.
Change packages here, validate, and merge through a PR. Then in a clean core
checkout import the reviewed full commit SHA:

```sh
python3 scripts/connectors/vendor.py --source /path/to/holler-connectors --commit FULL_COMMIT_SHA
python3 scripts/connectors/vendor.py --check
go test ./internal/connector
```

The core PR reviews the pinned commit, per-file hashes and any required compiled
manifest changes together. Do not edit the vendored copy directly. For a skill or
hook change, refresh its package hash and the matching core manifest together;
never bypass the manifest check. Runtime/permission changes require appropriate
live certification and operator trust, not just this repository's static tests.

## Distribution

Users continue to run `brew install 72olabs/tap/holler` and `holler setup codex`
or `holler setup claude`. Core packages its verified, pinned connector snapshot
under `share/holler/marketplace`; consumers do not clone this private repository.
Core CI is network-independent for this dependency and needs no cross-repo token.
Nothing in this repository automatically publishes a release or modifies a live
installation. OpenCode remains subject to its existing live-certification gate.
