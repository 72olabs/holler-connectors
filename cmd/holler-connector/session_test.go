package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/72olabs/holler-connectors/api"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// A wire fixture, not a daemon implementation: tests exercise the public
// command against explicit replies without importing core or opening SQLite.
func sessionFixture(t *testing.T, handle func(api.Request) (any, *api.RPCError, bool)) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "hc-session-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var active net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			active = c
			mu.Unlock()
			for {
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				var size uint32
				if binary.Read(c, binary.BigEndian, &size) != nil {
					break
				}
				if size > api.MaxFrameBytes {
					break
				}
				raw := make([]byte, size)
				if _, err := io.ReadFull(c, raw); err != nil {
					break
				}
				var req api.Request
				if json.Unmarshal(raw, &req) != nil {
					break
				}
				var result any
				var rpcErr *api.RPCError
				var drop bool
				if req.Op == "hello" {
					var hello map[string]any
					_ = json.Unmarshal(req.Args, &hello)
					result = map[string]any{"protocol": 1, "actor": hello["actor"], "run_id": hello["run_id"], "capabilities": []string{api.ActorAllocationCapability, api.OperatorConditionsCapability}}
				} else {
					result, rpcErr, drop = handle(req)
				}
				if drop {
					break
				}
				body, _ := json.Marshal(result)
				response, _ := json.Marshal(api.Response{ID: req.ID, OK: rpcErr == nil, Result: body, Error: rpcErr})
				if binary.Write(c, binary.BigEndian, uint32(len(response))) != nil {
					break
				}
				if _, err := c.Write(response); err != nil {
					break
				}
			}
			_ = c.Close()
			mu.Lock()
			active = nil
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		mu.Lock()
		if active != nil {
			_ = active.Close()
		}
		mu.Unlock()
		<-done
		_ = os.RemoveAll(dir)
	})
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HOLLER_SOCKET", socket)
	t.Setenv("HOLLER_ACTOR", "terminal")
	t.Setenv("HOLLER_RUN", "run")
	t.Setenv("HOLLER_NAME_MODE", "exact")
	t.Setenv("HOLLER_CONNECTOR_CONFIG", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("HOLLER_CLAUDE_ATTENTION", "hook-long-poll")
	return socket
}

func TestSessionLifecycleRehydratesAndEndsExactSession(t *testing.T) {
	for _, harness := range []string{"claude", "codex", "opencode"} {
		t.Run(harness, func(t *testing.T) { checkSessionLifecycle(t, harness) })
	}
}
func checkSessionLifecycle(t *testing.T, harness string) {
	t.Setenv("CODEX_HOME", "")
	t.Setenv("HOLLER_OPENCODE_SERVER", "http://127.0.0.1:4096")
	t.Setenv("HOLLER_CODEX_ATTENTION", "native-queue")
	t.Setenv("HOLLER_OPENCODE_ATTENTION", "native-prompt")
	var mu sync.Mutex
	var registrations []string
	var expired string
	claims := 0
	sessionFixture(t, func(req api.Request) (any, *api.RPCError, bool) {
		mu.Lock()
		defer mu.Unlock()
		var args map[string]any
		_ = json.Unmarshal(req.Args, &args)
		switch req.Op {
		case "register_session":
			registrations = append(registrations, args["session_id"].(string))
			return map[string]any{"project_id": "default"}, nil, false
		case "check_inbox":
			return []map[string]any{{"message_id": "pending", "available": true}}, nil, false
		case "record_hydration", "expire_registration":
			if req.Op == "expire_registration" {
				expired = args["session_id"].(string)
			}
			return map[string]any{}, nil, false
		case "list_conditions", "capabilities":
			return []any{}, nil, false
		case "claim", "ack":
			claims++
		}
		return nil, &api.RPCError{Code: "bad_request", Message: "fixture unsupported operation"}, false
	})
	for _, event := range []string{"startup", "resume", "clear", "compact"} {
		var out, stderr bytes.Buffer
		input := `{"session_id":"same-session","source":"` + event + `"}`
		if code := run(context.Background(), []string{"hook", "--harness", harness}, strings.NewReader(input), &out, &stderr); code != 0 || !strings.Contains(out.String(), "1 unread message") {
			t.Fatalf("%s: %d %s %s", event, code, out.String(), stderr.String())
		}
	}
	var out, stderr bytes.Buffer
	if code := run(context.Background(), []string{"session-end", "--harness", harness}, strings.NewReader(`{"session_id":"same-session"}`), &out, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(registrations) != 4 || expired != "same-session" || claims != 0 {
		t.Fatalf("registrations=%v expired=%s claims=%d", registrations, expired, claims)
	}
	for _, s := range registrations {
		if s != "same-session" {
			t.Fatal(registrations)
		}
	}
}

func TestMonitorReconnectsAndEmitsReferenceOnlyWake(t *testing.T) {
	var mu sync.Mutex
	waits, attaches, registrations := 0, 0, 0
	sessionFixture(t, func(req api.Request) (any, *api.RPCError, bool) {
		mu.Lock()
		defer mu.Unlock()
		switch req.Op {
		case "monitor_attach":
			attaches++
			if attaches == 1 {
				return nil, &api.RPCError{Code: "registration_expired"}, false
			}
			return map[string]any{}, nil, false
		case "register_session":
			registrations++
			return map[string]any{}, nil, false
		case "check_inbox":
			return []any{}, nil, false
		case "wait_attention":
			waits++
			if waits == 1 {
				return nil, nil, true
			}
			return map[string]any{"message_id": "wake-reference", "from_actor": "DO-NOT-INJECT", "thread_id": "DO-NOT-INJECT"}, nil, false
		}
		return nil, &api.RPCError{Code: "bad_request"}, false
	})
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	captured := make(chan string, 1)
	go func() { raw, _ := io.ReadAll(reader); captured <- string(raw) }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var out bytes.Buffer
	code := run(ctx, []string{"monitor", "--harness", "claude", "--wait", "50ms", "--startup-grace", "1ns"}, strings.NewReader(`{"session_id":"session"}`), &out, writer)
	_ = writer.Close()
	text := <-captured
	if code != 2 || !strings.Contains(text, "wake-reference") || strings.Contains(text, "DO-NOT-INJECT") {
		t.Fatalf("code=%d output=%s", code, text)
	}
	mu.Lock()
	defer mu.Unlock()
	if waits < 2 || attaches < 3 || registrations != 1 {
		t.Fatalf("no recovery: waits=%d attaches=%d", waits, attaches)
	}
}

func TestMonitorStopsForEndedOrSupersededSession(t *testing.T) {
	for _, errorCode := range []string{"session_ended", "presence_superseded", "binding_stale", "actor_adopted"} {
		t.Run(errorCode, func(t *testing.T) {
			sessionFixture(t, func(req api.Request) (any, *api.RPCError, bool) {
				return nil, &api.RPCError{Code: errorCode, Message: errorCode}, false
			})
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			done := make(chan []byte, 1)
			go func() { raw, _ := io.ReadAll(reader); done <- raw }()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var out bytes.Buffer
			code := run(ctx, []string{"monitor", "--harness", "claude"}, strings.NewReader(`{"session_id":"session"}`), &out, writer)
			_ = writer.Close()
			text := <-done
			if code != 0 || !strings.Contains(string(text), "stopped") {
				t.Fatalf("%d %s", code, text)
			}
		})
	}
}

func TestStopContinuationSuppressesOnlyPreviouslyReportedWake(t *testing.T) {
	var mu sync.Mutex
	waits := 0
	sessionFixture(t, func(req api.Request) (any, *api.RPCError, bool) {
		mu.Lock()
		defer mu.Unlock()
		switch req.Op {
		case "monitor_attach":
			return map[string]any{}, nil, false
		case "check_inbox":
			return []map[string]any{{"message_id": "old", "available": true}}, nil, false
		case "wait_attention":
			waits++
			return map[string]any{"message_id": "new"}, nil, false
		}
		return nil, &api.RPCError{Code: "bad_request"}, false
	})
	for _, tc := range []struct{ input, want string }{{`{"session_id":"s"}`, "old"}, {`{"session_id":"s","stop_hook_active":true}`, "new"}} {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		captured := make(chan string, 1)
		go func() { raw, _ := io.ReadAll(reader); captured <- string(raw) }()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		var out bytes.Buffer
		code := run(ctx, []string{"monitor", "--harness", "claude"}, strings.NewReader(tc.input), &out, writer)
		cancel()
		_ = writer.Close()
		text := <-captured
		_ = reader.Close()
		if code != 2 || !strings.Contains(text, "message "+tc.want) {
			t.Fatalf("want=%s code=%d output=%s", tc.want, code, text)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if waits != 1 {
		t.Fatalf("waits=%d", waits)
	}
}

func TestDuplicateMonitorCannotOwnSameSession(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "socket")
	first, ok, err := acquireMonitorLock(socket, "actor", "run", "session")
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	defer first.Close()
	second, ok, err := acquireMonitorLock(socket, "actor", "run", "session")
	if second != nil {
		second.Close()
	}
	if err != nil || ok {
		t.Fatalf("duplicate admitted: %v %v", ok, err)
	}
}

func TestResultChannelContextCancelsOnSocketClosure(t *testing.T) {
	descriptors, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	resultWriter := os.NewFile(uintptr(descriptors[0]), "result-writer")
	peer := os.NewFile(uintptr(descriptors[1]), "result-peer")
	defer resultWriter.Close()
	ctx, cancel, watched := resultChannelContext(context.Background(), resultWriter)
	defer cancel()
	if !watched {
		t.Fatal("socket result channel was not watched")
	}
	if err := peer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("result-channel closure did not cancel monitor context")
	}
}
