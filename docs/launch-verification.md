# Codex and Claude launch verification

This is the verification contract for HOL-15, HOL-69, HOL-70 and HOL-71. It is
not a live compatibility certificate. Source changes, static tests and a
successful installation are separate from proof that a real agent processes a
wake through MCP claim and acknowledgement.

## Source and version evidence

The starting connector source is `486108aaa277c92baa98a4ef74a60d3dfdee4a2e`,
imported by core `f24fbbcf6ca7953f14b0a0538433062f966ee3ba`. The controller
references below use daytona-runners
`dfe73abb9f8716058892467a5739cba5230fa683`. Record the final candidate commits,
archive checksum and selected clients in the hash-bound request after the
source changes are reviewed and committed.

| Evidence | Claude Code | Codex CLI |
| --- | --- | --- |
| Starting package manifest: minimum client | 2.1.247 | 0.149.1 |
| Starting package manifest: tested client | 2.1.259 | 0.151.0 |
| Local executable observed on 2026-11-02 | 2.1.314 | 0.166.0 |

The local versions are observations, not new support claims. The starting
minimum versions do not prove support for newly added hooks. Determine the
minimum version for each new hook from its supported client contract, test the
selected pair, and update the package and compiled core manifests together.
Never substitute `latest` for an exact version in a request.

Launch claims cover the tested **CLI modes**. Claude Desktop's Code tab,
Claude cloud sessions, the Codex app and Codex app-server routes require their
own certification. They do not inherit a CLI certificate. Claude Channels are
research-only and are absent from the shipping plugin; do not add `--channels`
or change organization policy to repair a packaged connector.

## Claude acceptance

The [existing failure report](https://github.com/72olabs/holler-connectors/issues/2)
is scoped to connector/release 0.8.0 with Claude Code 2.1.280. It does not prove
failure or success on the new candidate. Reproduce the first MCP claim/ack on
the selected candidate and client pins. If it passes, record the exact
artifact, scenario and evidence hash; if it fails, record a body-free failure
classification and fix or narrow the support claim before launch.

The packaged interactive route uses `asyncRewake` hooks on `SessionStart`,
`Stop` and `StopFailure`, plus `SessionEnd` cleanup. Print and SDK entrypoints
retain registration, hydration and MCP but skip the live monitor. Test those
paths separately:

- Cold startup hydrates queued work, then the real client claims and ACKs it.
- An armed interactive client wakes while idle without a new stdin prompt,
  claims the notified message and ACKs it.
- After normal and failed turns, the monitor rearms. Already surfaced notices
  do not produce a continuation loop; a new message still wakes the client.
- Daemon outage/restart reconnects safely. Claim expiry after a client crash
  redelivers the same message without duplicate successful processing.
- One-shot print/SDK calls exit after their response and can still use MCP;
  they must not be counted as evidence of interactive idle attention.

Core's existing deterministic tests cover the reference-only monitor output,
Stop continuation suppression, a new wake after suppression, daemon outages,
stale identity and terminal monitor outcomes. Plugin tests cover print-mode
monitor suppression while retaining the lifecycle hook. These tests use
disposable fixtures and do not replace live client evidence:

- [Monitor CLI tests](https://github.com/72olabs/holler/blob/f24fbbcf6ca7953f14b0a0538433062f966ee3ba/cmd/holler/main_test.go)
- [Claude plugin wrapper tests](https://github.com/72olabs/holler/blob/f24fbbcf6ca7953f14b0a0538433062f966ee3ba/internal/connector/claude_plugin_test.go)
- [Certification tests](https://github.com/72olabs/holler/blob/f24fbbcf6ca7953f14b0a0538433062f966ee3ba/internal/connector/certify_test.go)

## Codex and shared acceptance

For HOL-69, prove pickup during a long tool-using turn and turn-end pickup only
when addressed, claimable work remains. Preserve user review of every
executable hook. If the supported client cannot register before its first
submitted prompt, expose and document that limit rather than promising wake
from an untouched TUI.

[Core issue #38](https://github.com/72olabs/holler/issues/38) concerns obsolete
queue notices that start inference after their messages were already drained.
Measure model turns for a busy-recipient burst and a bounded idle soak, not
just daemon acceptance or visible assistant text. New arrivals during or after
a drain, partial drains, lease expiry, NACK, crash/restart and separate
actors/endpoints must retain attention. Legacy and managed deliveries must not
suppress each other. Report any remaining queue-consumption race explicitly.

For HOL-70, run adversarial fixtures through each agent-readable message path
and hook context. Message bodies must remain data in an untrusted envelope;
peer-controlled titles, topics, names, aliases and condition subjects must not
forge Holler's framing. Repeat the real Claude and Codex MCP canaries after the
schema and guidance change. Only MCP claim followed by MCP ACK is processing
evidence; a hook, queue submission or HTTP success is an attempted wake.

## Existing canary coverage and gaps

| Scenario | What it proves on the selected artifact and clients |
| --- | --- |
| C0 | Packaging/build/client pins, doctor, onboarding, Codex hook trust and Claude startup hook; zero model turns |
| C1 | Offline hydration and real MCP claim/ack |
| C2 | Interactive bidirectional idle wake and correlated processing |
| C3 | Daemon restart, client reconnect and durable delivery |
| C4 | Claim crash, lease expiry and redelivery |
| C8 | Minimum/tested client cold and MCP paths; does not by itself prove a new mid-turn hook |
| C10 | Managed client recovery and narrow claim/ack tools |
| C11 | Managed targeted idle wakes and no wake/consumption by non-targeted members |
| C12 | Three real sessions, selective delivery, thread isolation and restart |
| C20 | Synthetic-recipient burst with live Claude/Codex participation; does not measure stale Codex inference turns |

At the controller revision above, C0 and the scale-out compatibility attestation
require **exactly two** Codex trust entries: `session_start` and `session_end`.
Before certifying a package with more executable hooks, update the controller's
review, entry validation and attestation tests to verify the exact packaged
hook set. Do not weaken trust checks or populate the operator's trust store to
make an old controller pass. See the
[controller worker](https://github.com/72olabs/daytona-runners/blob/dfe73abb9f8716058892467a5739cba5230fa683/products/holler/worker.py).

The existing catalog does not provide a dedicated live proof of Codex mid-turn
pickup, suppression of stale inference turns, or Claude `StopFailure` rearming.
Add committed bounded scenarios with independent processing/turn-count oracles
before claiming those behaviors. C24 tests Claude development-channel
feasibility, not this packaged hook route. Scenario definitions and fake-driver
passes are preparation evidence only.

## Prepare and execute the final check

1. Validate connector assets, shell syntax, identities and package hashes:

   ```sh
   python3 scripts/validate.py
   python3 -m unittest discover -s tests -v
   ```

2. Import the reviewed connector commit through core's vendor script, update
   the matching compiled manifests and verify the lock offline. Run the
   affected core tests and release packaging checks. The product tree supplied
   to the canary must be committed and clean; an uncommitted archive of source
   is not a candidate identity.

3. From a clean, committed `daytona-runners` checkout, use the primary serial
   `compatibility-01` lane. Follow its `AGENTS.md` and
   [lane contract](https://github.com/72olabs/daytona-runners/blob/dfe73abb9f8716058892467a5739cba5230fa683/products/holler/LANES.md).
   One active coding agent owns a lane. Do not read provider credentials,
   client configuration, transcripts or generated `.runs/` contents. Leave
   low-cost models and per-run budgets at their committed defaults. The
   controller's [bounded campaign workflow](https://github.com/72olabs/daytona-runners/blob/dfe73abb9f8716058892467a5739cba5230fa683/products/holler/CAMPAIGNS.md)
   is the default for Daytona and real-client tests.

4. After choosing exact client versions, pass the same arguments to doctor,
   check, campaign preparation and checkpoint. This example selects the local observed pair as a
   **compatibility target**, not as a certified replacement for the baseline:

   ```sh
   python3 products/holler/harness.py doctor \
     --product-repo /absolute/path/to/committed-holler-candidate \
     --lane compatibility-01 --claude-version 2.1.314 --codex-version 0.166.0

   python3 products/holler/harness.py check \
     --product-repo /absolute/path/to/committed-holler-candidate \
     --lane compatibility-01 --product candidate --tier core \
     --claude-version 2.1.314 --codex-version 0.166.0

   ```

5. Use `campaign prepare` to create a local, credential-free proposal after the
   code and local checks are ready. Pin the reviewed controller commit, final
   candidate root, exact clients/models, lane, scenarios and permitted product
   paths. Include expiry and cumulative limits for runs, model turns, Claude
   USD accounting, Codex reported tokens, worker seconds, builder count and
   builder seconds. Choose limits that fit at least one complete reservation;
   failures and interrupted attempts consume the full reservation. Preparing
   the file authorizes no execution.

   Show that concrete envelope, campaign hash and its printed
   `operator_approval_command` to the operator. The operator runs the command in
   their own terminal and types the campaign id. Agents must not allocate a
   PTY, pipe input or invoke internal approval APIs to impersonate that action.
   General permission to implement these tickets is not approval of the
   future campaign. After approval, in-scope checkpoints and fixes/retries use
   the campaign id without repeated child-request approvals:

   ```sh
   python3 products/holler/harness.py checkpoint \
     --product-repo /absolute/path/to/committed-holler-candidate \
     --lane compatibility-01 --product candidate --tier core \
     --claude-version 2.1.314 --codex-version 0.166.0 \
     --campaign connectors-launch --execute
   ```

   `connectors-launch` must be the actual approved campaign id. The example is
   not a grant. Every child request still binds exact candidate/artifact
   bytes. A changed controller, scope, clients/models, lanes or exhausted
   budget needs a new reviewed campaign; do not reset campaign state or fall
   back to per-request approval. Use the legacy exact-request workflow only
   when the operator explicitly requests that exception for a particular test.
   Missing credentials, lane contention or exhausted capacity are
   infrastructure blockers, not connector failures.

6. Run additional committed feature scenarios in separate bounded checkpoints
   rather than exceeding the core budget. A feature checkpoint does not
   replace the release tier, which also needs the controller's pinned upgrade
   artifact/version/schema inputs. Stop the owned runner after execution and
   delete disposable builders, including on failure.

7. Record candidate and controller SHAs, archive checksum, client versions,
   scenario selection, request/evidence hashes, status, usage when exposed,
   and remaining limits. `connector doctor` stops at `CONFIGURED`; only
   correlated real-client `connector certify` evidence can report `READY`.
   Linux/Daytona results do not establish Homebrew install/upgrade behavior;
   record the disposable macOS release check separately.

## Candidate changes prepared for review

The 0.8.3 candidate adds four reviewed Codex executable hooks: SessionStart,
SessionEnd, PostToolUse and Stop. Its conservative Codex floor is 0.166.0;
`tested_client_version` remains `pending-live-certification` until real-client
evidence exists. Historical Claude test pins remain historical evidence.

The corresponding controller change validates the exact packaged two-hook
legacy or four-hook delivery contract before driving the real review UI, and
binds the resulting hashes to the artifact in C0/scale-out attestation. It pins
stopped-database inspection to schema 23 and its managed tables.

New C25 is a bounded one-turn live Codex probe: inject synthetic mail during
a read-only shell tool sequence, require MCP acknowledgement before the second
tool boundary, and inspect the private PostToolUse receipt after the client and
daemon stop. C25 still needs an approved live run; deterministic controller
tests establish the oracle behavior, not client compatibility. It does not
measure residual already-queued empty inference turns.

Do not release this candidate based on package, unit or fake-controller tests.
The Claude reproduction, current-version live wake/claim/ACK, C25, and residual
wake measurement remain distinct acceptance evidence.
