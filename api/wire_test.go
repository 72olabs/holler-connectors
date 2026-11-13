package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/72olabs/holler-connectors/bus"
)

func TestFrameRoundTripAndLayout(t *testing.T) {
	var buffer bytes.Buffer
	request := Request{ID: 7, Op: "ping", Args: json.RawMessage(`{}`)}
	if err := WriteFrame(&buffer, request); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(request)
	if got := binary.BigEndian.Uint32(buffer.Bytes()[:4]); int(got) != len(payload) {
		t.Fatalf("length prefix = %d, want %d", got, len(payload))
	}
	if !bytes.Equal(buffer.Bytes()[4:], payload) {
		t.Fatalf("frame body = %q, want %q", buffer.Bytes()[4:], payload)
	}
	var decoded Request
	if err := ReadFrame(&buffer, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != 7 || decoded.Op != "ping" || string(decoded.Args) != `{}` {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestReadFrameIsStrict(t *testing.T) {
	frame := func(body string) *bytes.Buffer {
		var buffer bytes.Buffer
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(body)))
		buffer.Write(length[:])
		buffer.WriteString(body)
		return &buffer
	}
	var request Request
	if err := ReadFrame(frame(`{"id":1,"op":"ping","extra":true}`), &request); err == nil || !strings.Contains(err.Error(), `unknown field "extra"`) {
		t.Fatalf("unknown field: %v", err)
	}
	if err := ReadFrame(frame(`{"id":1,"op":"ping"}{}`), &request); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
	if err := ReadFrame(frame(``), &request); err == nil {
		t.Fatal("zero-length frame was accepted")
	}
	var oversized bytes.Buffer
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], MaxFrameBytes+1)
	oversized.Write(length[:])
	if err := ReadFrame(&oversized, &request); err == nil {
		t.Fatal("oversized frame length was accepted")
	}
	if err := WriteFrame(&bytes.Buffer{}, strings.Repeat("x", MaxFrameBytes)); err == nil {
		t.Fatal("oversized frame was written")
	}
}

func TestFinalizesProvisionalActor(t *testing.T) {
	for _, op := range []string{"record_agent_client", "ping", "list_events", "who", "list_aliases", "resolve_alias", "list_capabilities", "invoke_read_capability", "live_registrations", "heartbeat_registrations", "list_conditions"} {
		if FinalizesProvisionalActor(op) {
			t.Errorf("%s finalizes a provisional actor", op)
		}
	}
	for _, op := range []string{"send", "claim", "ack", "register_session", "set_alias", "invoke_write_capability", "some_future_op"} {
		if !FinalizesProvisionalActor(op) {
			t.Errorf("%s leaves a provisional actor provisional", op)
		}
	}
}

// errorDaemon answers hello, then answers every operation with the given
// error codes in turn.
func errorDaemon(t *testing.T, helloError *RPCError, callErrors ...*RPCError) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "holler-api-decoder-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	listener, err := net.Listen("unix", filepath.Join(directory, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		request, err := readRequest(connection)
		if err != nil {
			return
		}
		if helloError != nil {
			_ = writeResponse(connection, Response{ID: request.ID, Error: helloError})
			return
		}
		result, _ := json.Marshal(map[string]interface{}{"protocol": ProtocolVersion, "actor": "alice", "run_id": "run-1"})
		_ = writeResponse(connection, Response{ID: request.ID, OK: true, Result: result})
		for _, callError := range callErrors {
			request, err := readRequest(connection)
			if err != nil {
				return
			}
			if callError == nil {
				result, _ := json.Marshal(map[string]string{"echo": request.Op})
				_ = writeResponse(connection, Response{ID: request.ID, OK: true, Result: result})
				continue
			}
			_ = writeResponse(connection, Response{ID: request.ID, Error: callError})
		}
	}()
	return listener.Addr().String()
}

type customError struct{ code string }

func (e *customError) Error() string { return "custom " + e.code }

func TestErrorDecoderIsConsultedFirstAndFallsThrough(t *testing.T) {
	var seen []string
	decoder := WithErrorDecoder(func(rpc *RPCError) error {
		seen = append(seen, rpc.Code)
		if rpc.Code == "custom_code" || rpc.Code == "not_found" {
			return &customError{code: rpc.Code}
		}
		return nil
	})
	socket := errorDaemon(t, nil,
		nil,
		&RPCError{Code: "custom_code", Message: "custom message"},
		&RPCError{Code: "not_found", Message: "shadowed"},
		&RPCError{Code: "alias_not_found", Message: bus.ErrAliasNotFound.Error()},
		&RPCError{Code: "other_code", Message: "plain message"},
	)
	client, err := Dial(context.Background(), socket, Identity{Actor: "alice", RunID: "run-1", Client: "test"}, decoder)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()

	var result struct {
		Echo string `json:"echo"`
	}
	if err := client.Call(ctx, "custom_op", map[string]string{"k": "v"}, &result); err != nil || result.Echo != "custom_op" {
		t.Fatalf("Call result = %+v, %v", result, err)
	}
	var custom *customError
	if err := client.Call(ctx, "custom_op", nil, nil); !errors.As(err, &custom) || custom.code != "custom_code" {
		t.Fatalf("custom code = %v", err)
	}
	if err := client.Call(ctx, "custom_op", nil, nil); !errors.As(err, &custom) || custom.code != "not_found" || errors.Is(err, bus.ErrNotFound) {
		t.Fatalf("decoder did not take precedence over the client's mapping: %v", err)
	}
	if err := client.Call(ctx, "custom_op", nil, nil); !errors.Is(err, bus.ErrAliasNotFound) {
		t.Fatalf("decoder returning nil did not fall through to the client's mapping: %v", err)
	}
	if err := client.Call(ctx, "custom_op", nil, nil); err == nil || err.Error() != "plain message" {
		t.Fatalf("unknown code = %v", err)
	}
	if strings.Join(seen, ",") != "custom_code,not_found,alias_not_found,other_code" {
		t.Fatalf("decoder saw %v", seen)
	}
}

func TestErrorDecoderAppliesToHello(t *testing.T) {
	socket := errorDaemon(t, &RPCError{Code: "custom_code", Message: "refused"})
	_, err := Dial(context.Background(), socket, Identity{Actor: "alice", RunID: "run-1", Client: "test"},
		WithErrorDecoder(func(rpc *RPCError) error { return &customError{code: rpc.Code} }))
	var custom *customError
	if !errors.As(err, &custom) || custom.code != "custom_code" {
		t.Fatalf("hello error = %v", err)
	}
}
