// Command holler-connector is the open-source client for a separately
// installed Holler daemon. Its commands are in package cli.
package main

import "github.com/72olabs/holler-connectors/cli"

func main() {
	cli.Main(options)
}

var options = cli.Options{
	Usage: usage,
	Commands: map[string]cli.Command{
		"notify": cli.Notify,
	},
}

const usage = `holler-connector: open-source client for a separately installed Holler daemon

Usage:
 holler-connector notify < notification.json
 holler-connector version | status
 holler-connector connector setup --harness claude|codex|opencode [--apply] [options]
 holler-connector connector launch --harness claude|codex|opencode -- [harness arguments]
 holler-connector connector manifest|doctor|certify --harness claude|codex|opencode
 holler-connector hook|session-end --harness claude|codex|opencode
 holler-connector monitor --harness claude
 holler-connector codex-attention --event PostToolUse|Stop
 holler-connector mcp [--socket PATH] [--actor ACTOR --run RUN]
 holler-connector send|inbox|claim|ack|extend|nack|events [options]
 holler-connector who|profile|alias|adopt|actor|conditions|migrate [options]

Use COMMAND --help for flags. HOLLER_SOCKET selects the daemon Unix socket.
This executable does not start a daemon, open its database, or manage desktop agents.`
