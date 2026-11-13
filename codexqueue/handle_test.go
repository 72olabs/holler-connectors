package codexqueue_test

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/72olabs/holler-connectors/bus"
	"github.com/72olabs/holler-connectors/codexqueue"
)

func TestHomeHandleRoundTripAndLegacy(t *testing.T) {
	home := codexqueue.SyntheticCodexHome(t)
	value, err := codexqueue.EncodeHandle(codexqueue.Handle{Thread: "thread", CodexHome: home})
	if err != nil {
		t.Fatal(err)
	}
	h, err := codexqueue.DecodeHandle(value)
	if err != nil || h.Version != 2 || h.Thread != "thread" || h.CodexHome != home {
		t.Fatalf("handle=%+v err=%v", h, err)
	}
	if err := codexqueue.ValidateRegistrationHandle(value, "other-thread"); !errors.Is(err, codexqueue.ErrInvalidHandle) {
		t.Fatal(err)
	}
	for _, legacy := range []string{"thread-1", "codex-thread-1", "an exact session name"} {
		h, err := codexqueue.DecodeHandle(legacy)
		if err != nil || h != (codexqueue.Handle{Thread: legacy}) {
			t.Fatalf("legacy=%+v err=%v", h, err)
		}
	}
	if _, err := codexqueue.EncodeHandle(codexqueue.Handle{Thread: "t", CodexHome: home, Endpoint: "unix:///private/socket"}); !errors.Is(err, codexqueue.ErrUnsupportedEndpoint) {
		t.Fatal(err)
	}
}

func TestHomeHandleRejectsMalformedAndUnrecognizedRouting(t *testing.T) {
	encode := func(raw string) string { return "codex-v2:" + base64.RawURLEncoding.EncodeToString([]byte(raw)) }
	for _, value := range []string{"", "codex-v3:anything", "codex-unix-v1:abc", strings.Repeat("x", 4097), "t\n", "codex-v2:!", encode(`{"v":3,"thread":"t"}`), encode(`{"v":2,"thread":"t","API_KEY":"no"}`), encode(`{"v":2,"thread":"t"} {}`), encode(`{"v":2,"thread":"t","endpoint":"unix:///tmp/s"}`)} {
		if _, err := codexqueue.DecodeHandle(value); err == nil {
			t.Fatalf("accepted malformed handle of length %d", len(value))
		}
	}
}

func TestHomeValidationRejectsUnsafePathsWithoutDisclosingThem(t *testing.T) {
	home := codexqueue.SyntheticCodexHome(t)
	link := filepath.Join(filepath.Dir(home), "link")
	if err := os.Symlink(home, link); err != nil {
		t.Fatal(err)
	}
	parentLink := filepath.Join(filepath.Dir(home), "parent-link")
	if err := os.Symlink(filepath.Dir(home), parentLink); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, "config.toml")
	for _, path := range []string{"relative", home + "/../role home", home + "/", home + "\n", home + "\x00", home + "/missing", file, link, filepath.Join(parentLink, "role home"), "/" + strings.Repeat("s", 1024)} {
		if err := codexqueue.ValidateHome(path); !errors.Is(err, codexqueue.ErrInvalidHome) || strings.Contains(err.Error(), home) {
			t.Fatalf("unsafe path diagnostic=%v", err)
		}
	}
	for _, mode := range []os.FileMode{0777, 0770, 0755, 0750} {
		if err := os.Chmod(home, mode); err != nil {
			t.Fatal(err)
		}
		if err := codexqueue.ValidateHome(home); !errors.Is(err, codexqueue.ErrInvalidHome) {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := codexqueue.ValidateHome(home); err != nil {
		t.Fatal(err)
	}
	// Validation must not open the credential symlink; a missing target is fine.
	if err := os.Remove(filepath.Join(filepath.Dir(home), "synthetic-auth.json")); err != nil {
		t.Fatal(err)
	}
	if err := codexqueue.ValidateHome(home); err != nil {
		t.Fatal(err)
	}
}

func TestHomeValidationRejectsForeignOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to create a foreign-uid fixture")
	}
	home := codexqueue.SyntheticCodexHome(t)
	if err := os.Chown(home, 65534, -1); err != nil {
		t.Skip("cannot change fixture ownership")
	}
	t.Cleanup(func() { _ = os.Chown(home, 0, -1) })
	if err := codexqueue.ValidateHome(home); !errors.Is(err, codexqueue.ErrInvalidHome) {
		t.Fatal(err)
	}
}

func TestHomeAdapterUsesOneBoundHomeAndFixedArguments(t *testing.T) {
	home := codexqueue.SyntheticCodexHome(t)
	t.Setenv("CODEX_HOME", "/daemon-home-not-session-home")
	t.Setenv("HOLLER_TEST_DAEMON_VARIABLE", "daemon-owned")
	value, err := codexqueue.EncodeHandle(codexqueue.Handle{Thread: "thread", CodexHome: home})
	if err != nil {
		t.Fatal(err)
	}
	var gotEnv, gotArgs []string
	adapter := codexqueue.New("fake-codex", time.Second, func(_ context.Context, name string, env []string, args ...string) (string, string, int, error) {
		if name != "fake-codex" {
			t.Fatal(name)
		}
		gotEnv, gotArgs = env, args
		return "", "", 0, nil
	})
	registration := bus.Registration{SessionID: "thread", DeliveryHandle: value}
	if detail, ok := adapter.Notify(context.Background(), registration, bus.Message{ID: "message"}); !ok || detail != codexqueue.Name {
		t.Fatalf("detail=%s ok=%v", detail, ok)
	}
	count := 0
	for _, entry := range gotEnv {
		if strings.HasPrefix(entry, "CODEX_HOME=") {
			count++
			if entry != "CODEX_HOME="+home {
				t.Fatal("wrong home")
			}
		}
	}
	if count != 1 || !strings.Contains(strings.Join(gotEnv, "\n"), "HOLLER_TEST_DAEMON_VARIABLE=daemon-owned") {
		t.Fatal("wrong environment")
	}
	if len(gotArgs) != 5 || !reflect.DeepEqual(gotArgs[:4], []string{"queue", "--thread", "thread", "--message"}) || strings.Contains(strings.Join(gotArgs, " "), home) {
		t.Fatalf("args=%v", gotArgs)
	}
	// The original plain-thread path still inherits the daemon environment.
	if _, ok := adapter.Notify(context.Background(), bus.Registration{DeliveryHandle: "legacy"}, bus.Message{ID: "m"}); !ok || gotEnv != nil {
		t.Fatal("legacy routing changed")
	}
}

func TestHomeAdapterRechecksBeforeEveryWakeAndNeverFallsBack(t *testing.T) {
	home := codexqueue.SyntheticCodexHome(t)
	value, err := codexqueue.EncodeHandle(codexqueue.Handle{Thread: "t", CodexHome: home})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	adapter := codexqueue.New("fake", time.Second, func(context.Context, string, []string, ...string) (string, string, int, error) {
		calls++
		return "", "", 0, nil
	})
	if detail, ok := adapter.Notify(context.Background(), bus.Registration{SessionID: "other", DeliveryHandle: value}, bus.Message{}); ok || detail != "codex_handle_invalid" || calls != 0 {
		t.Fatal(detail, calls)
	}
	if err := os.Chmod(home, 0770); err != nil {
		t.Fatal(err)
	}
	if detail, ok := adapter.Notify(context.Background(), bus.Registration{SessionID: "t", DeliveryHandle: value}, bus.Message{}); ok || detail != "codex_home_invalid" || calls != 0 {
		t.Fatal(detail, calls)
	}
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	adapter = codexqueue.New("fake", time.Second, func(context.Context, string, []string, ...string) (string, string, int, error) {
		return "", "failure at " + home, 1, errors.New("private path " + home)
	})
	if detail, ok := adapter.Notify(context.Background(), bus.Registration{SessionID: "t", DeliveryHandle: value}, bus.Message{}); ok || detail != "codex_queue_failed" {
		t.Fatal(detail)
	}
}
