package codexqueue

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const handlePrefix = "codex-v2:"

var (
	ErrInvalidHandle       = errors.New("codex_handle_invalid")
	ErrInvalidHome         = errors.New("codex_home_invalid")
	ErrUnsupportedEndpoint = errors.New("codex_endpoint_unsupported")
)

// Handle is a private routing capability, never an agent profile or message body.
// Endpoint is reserved for separately reviewed endpoint routing; it currently
// fails closed rather than falling back to the default app server.
type Handle struct {
	Version   int    `json:"v"`
	Thread    string `json:"thread"`
	CodexHome string `json:"codex_home"`
	Endpoint  string `json:"endpoint,omitempty"`
}

func EncodeHandle(h Handle) (string, error) {
	h.Version = 2
	if err := validateHandle(h); err != nil {
		return "", err
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return "", ErrInvalidHandle
	}
	value := handlePrefix + base64.RawURLEncoding.EncodeToString(raw)
	if len(value) > 4096 {
		return "", ErrInvalidHandle
	}
	return value, nil
}

func DecodeHandle(value string) (Handle, error) {
	if len(value) > 4096 || strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\r\n") {
		return Handle{}, ErrInvalidHandle
	}
	if !strings.HasPrefix(value, handlePrefix) {
		if strings.HasPrefix(value, "codex-v") || strings.HasPrefix(value, "codex-unix-") {
			return Handle{}, ErrInvalidHandle
		}
		return Handle{Thread: value}, nil // Existing registrations retain their routing.
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, handlePrefix))
	if err != nil {
		return Handle{}, ErrInvalidHandle
	}
	var h Handle
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(&h) != nil {
		return Handle{}, ErrInvalidHandle
	}
	if d.Decode(new(interface{})) != io.EOF {
		return Handle{}, ErrInvalidHandle
	}
	if err := validateHandle(h); err != nil {
		return h, err
	}
	return h, nil
}

// ValidateRegistrationHandle also protects direct registrations that bypass the
// lifecycle hook. No client-supplied path is included in returned diagnostics.
func ValidateRegistrationHandle(value, sessionID string) error {
	h, err := DecodeHandle(value)
	if h.Version == 2 && h.Thread != sessionID {
		return ErrInvalidHandle
	}
	return err
}

func validateHandle(h Handle) error {
	if h.Version != 2 || h.Thread == "" || strings.TrimSpace(h.Thread) != h.Thread || len(h.Thread) > 1024 || strings.ContainsAny(h.Thread, "\x00\r\n") {
		return ErrInvalidHandle
	}
	if h.Endpoint != "" {
		return ErrUnsupportedEndpoint
	}
	return ValidateHome(h.CodexHome)
}

// ValidateHome checks only the routing directory, not credentials or its
// contents. The runtime owns bundle/config validation and auth symlink policy.
// Recheck on each wake, since the home can disappear or change permissions.
func ValidateHome(home string) error {
	if home == "" || len(home) > 1024 || !filepath.IsAbs(home) || filepath.Clean(home) != home || strings.ContainsAny(home, "\x00\r\n") {
		return ErrInvalidHome
	}
	info, err := os.Lstat(home)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || !ownedByDaemon(info) {
		return ErrInvalidHome
	}
	real, err := filepath.EvalSymlinks(home)
	if err != nil || real != home {
		return ErrInvalidHome
	}
	return nil
}

func HandleFromEnvironment(sessionID string) (string, error) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		return "", nil
	}
	userHome, err := os.UserHomeDir()
	if err == nil && isDefaultHome(home, filepath.Join(userHome, ".codex")) {
		return "", nil
	}
	return EncodeHandle(Handle{Thread: sessionID, CodexHome: home})
}

func isDefaultHome(home, defaultHome string) bool {
	if filepath.Clean(home) == defaultHome {
		return true
	}
	realHome, homeErr := filepath.EvalSymlinks(home)
	realDefault, defaultErr := filepath.EvalSymlinks(defaultHome)
	return homeErr == nil && defaultErr == nil && realHome == realDefault
}

// UnavailableHomeHandle carries no path and deliberately cannot route a wake.
// A hook uses it with startup-only after invalid custom-home validation, so
// durable messaging still registers. ValidateRegistrationHandle reports
// ErrInvalidHome for discovery even while attention is startup-only.
func UnavailableHomeHandle(sessionID string) string {
	raw, _ := json.Marshal(Handle{Version: 2, Thread: sessionID})
	return handlePrefix + base64.RawURLEncoding.EncodeToString(raw)
}

func queueEnvironment(home string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "CODEX_HOME=") {
			env = append(env, entry)
		}
	}
	return append(env, "CODEX_HOME="+home)
}
