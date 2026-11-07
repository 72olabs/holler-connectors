# Public connector verification

No private checkout is required for these steps.

1. Run `./scripts/check.sh` for package validation, fixture tests, dependency
   boundaries, Go tests/vet, and the executable build. Run `go test -race ./...`
   for concurrency changes. Tests use temporary homes and synthetic data.
2. With a separately obtained core binary, run
   `python3 scripts/smoke-core.py --daemon /absolute/path/to/hollerd --external-wakes`.
   This verifies the binary interface in an isolated home, not real harness waking.
3. For live testing, use a disposable home and synthetic messages with the exact
   harness version being advertised. Record connector/core versions and checksums,
   harness versions, OS, mode, and pass/fail for each scenario:
   - Cold session startup and MCP claim/ack.
   - Resume and compact hydration without consuming messages early.
   - Idle wake, tool completion, and next-turn pickup with no duplicate loop.
   - Daemon restart/reconnect and delivery after reconnect.
   - Session end and replacement: the old session must stop receiving wakes.
   - Startup-only fallback when live attention is unavailable.
4. Report bounded failure labels and reproduction steps. Never upload delivery
   handles, raw hook requests, credentials, environment dumps, or transcripts.

Claim only the modes and versions actually tested. A successful notification
means the harness accepted a wake, not that the agent processed the message.
Record correlated claim/ack evidence separately. Managed desktop-agent recovery
is outside this terminal-session preview.
