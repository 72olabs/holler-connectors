package connector

import "strings"

// HostApp names the app a session runs in, from two variables of the environment
// the harness started in: the macOS app bundle identifier first, then TERM_PROGRAM.
// Only a name from bus.HostApps is returned; anything else reports nothing.
func HostApp(getenv func(string) string) string {
	switch strings.ToLower(strings.TrimSpace(getenv("__CFBundleIdentifier"))) {
	case "com.apple.terminal":
		return "terminal"
	case "com.googlecode.iterm2":
		return "iterm"
	case "com.t3tools.t3code":
		return "t3code"
	case "com.microsoft.vscode", "com.microsoft.vscodeinsiders":
		return "vscode"
	case "com.todesktop.230313mzl4w4u92":
		return "cursor"
	case "dev.warp.warp-stable":
		return "warp"
	case "com.mitchellh.ghostty":
		return "ghostty"
	case "dev.zed.zed":
		return "zed"
	}
	switch strings.TrimSpace(getenv("TERM_PROGRAM")) {
	case "Apple_Terminal":
		return "terminal"
	case "iTerm.app":
		return "iterm"
	case "vscode":
		if getenv("CURSOR_TRACE_ID") != "" {
			return "cursor"
		}
		return "vscode"
	case "WarpTerminal":
		return "warp"
	case "ghostty":
		return "ghostty"
	case "zed":
		return "zed"
	}
	return ""
}
