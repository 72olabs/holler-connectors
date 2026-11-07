# Contributing

Run `./scripts/check.sh` before submitting a change. Use `go test -race ./...`
for concurrency changes. Include a reproducible failure, harness version, expected
behavior, and tests. Redact private paths, tokens, transcript contents, and delivery
handles from reports. Never include user configuration or runtime databases.

To add a harness, implement lifecycle registration/context injection and an
attention adapter, supply plugin metadata and setup/doctor support, and add tests
using a fake harness. Declare startup-only fallback when live waking is absent.
Keep message bodies behind MCP claim/ack; a wake carries a reference only.

Changing packaged assets requires updating their package hashes and the frozen
Go manifests together. The Python and Go checks enforce this. Changes to identity,
permissions, or wake semantics require live certification before a release;
fixture tests alone do not establish vendor trust behavior.

New daemon capabilities need an explicit version/capability contract and a
negative compatibility test. Never add a private core module, database driver,
or local replacement dependency to make a connector test pass.

Contributions to this repository are provided under Apache-2.0. The separately
distributed Holler core retains its own license.
