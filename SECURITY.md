# Security policy

Report suspected vulnerabilities privately to **security@72olabs.ai**. Do not
open a public issue with exploit details, credentials, or private transcripts.
Include the affected connector commit/version, core version, harness version,
impact, and minimal reproduction with synthetic data. We will coordinate fixes
and disclosure with the reporter; no response-time guarantee is currently offered.

This repository is a source preview. Security fixes target the current main
branch; there are no supported maintenance branches yet. Core authorization and
storage remain in the separately distributed Holler core. Report issues spanning
that boundary to the same address.

Delivery handles can contain credentials. Never attach raw hook input, notification
requests, environment dumps, runtime databases, or user configuration to an issue.
