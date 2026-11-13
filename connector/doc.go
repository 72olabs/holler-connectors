// Package connector is the agent side of a Holler connector: the lifecycle
// hooks that register a harness session and build its start-up context
// (Runtime), identity resolution (RuntimeBinding), the settings files setup
// writes, setup and removal for Claude Code, Codex and OpenCode, doctor,
// certification, launch, and the frozen capability manifests that each
// plugin's connector.json repeats (Manifest, ConnectorVersion, PackageHash).
//
// It reaches the daemon only through the Store and EventReader interfaces it is
// given; api.Client satisfies both. It never delivers wakes: the daemon does
// that, through the adapters in codexqueue, opencodeprompt and notify.
//
// It is supported public API: the Holler daemon imports it too.
package connector
