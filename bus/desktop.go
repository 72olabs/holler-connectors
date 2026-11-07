package bus

import "strings"

// Desktop host tags distinguish app configurations within the existing local
// OS-user trust boundary. They are not process proof or remote credentials.
func ValidateDesktopHost(host string) error {
	if len(host) == 0 || len(host) > 64 || strings.Trim(host, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return &ValidationError{Field: "desktop_host", Problem: "must be 1-64 lowercase letters, digits or hyphens"}
	}
	return nil
}

// Encode the app scope in the desktop-only session namespace. This preserves
// independent run cleanup without changing the on-disk registration schema.
func DesktopSessionID(host, run string) string { return "desktop-app:" + host + ":" + run }

func DesktopSessionHost(session string) string {
	parts := strings.SplitN(session, ":", 3)
	if len(parts) != 3 || parts[0] != "desktop-app" || parts[2] == "" || ValidateDesktopHost(parts[1]) != nil {
		return ""
	}
	return parts[1]
}
