// Package mcp is the Model Context Protocol stdio server agents use to reach
// Holler: the tool definitions, their ToolSurfaceHash, and the envelope that
// marks message content from other agents as untrusted. It talks to the daemon
// only through the Store interface it is given.
//
// It is supported public API: the Holler daemon imports it too.
package mcp
