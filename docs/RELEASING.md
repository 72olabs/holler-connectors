# Release preparation

1. Run `./scripts/check.sh` and `go test -race ./...` on macOS and Linux.
2. Build a pinned core artifact separately and run `scripts/smoke-core.py` with
   that binary. Record its version/hash and connector commit in release evidence.
3. Verify live lifecycle and wake behavior for each advertised harness version,
   including reconnect, compact/resume, session end, and startup-only fallback.
   Use the companion core adapter handoff for Codex/OpenCode when certifying
   independently patched adapters.
4. Run `python3 scripts/package.py --version VERSION`. The archive includes only
   the connector binary, plugin assets, license, notice, and README. Publish
   checksums alongside each archive. Build on each supported OS/architecture.
5. Review the repository's full history and tracked files before making it public.
   This change does not publish a repository or artifact. Confirm dependency
   notices and the intended Apache-2.0 grant for the extracted first-party source.
6. Publish only after the companion core version and compatibility evidence are
   available. Until then describe this as a source preview, not a certified release.

Open-source runtime changes do not automatically update core's pinned plugin
snapshot. Coordinate that pin through core's existing vendor/check process.
Future core releases may consume the public adapter packages directly; the
subprocess protocol already permits independent binary replacement without a
Go module dependency on private source.
