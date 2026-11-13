package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/72olabs/holler-connectors/bus"
)

type clientMetadataStore struct {
	resultStore
	actor, run string
	client     bus.AgentClient
	err        error
}

func (s *clientMetadataStore) RecordAgentClient(_ context.Context, actor, run string, c bus.AgentClient) error {
	s.actor, s.run, s.client = actor, run, c
	return s.err
}

func TestInitializeCollectsOnlyClientMetadataAndFailureIsNonfatal(t *testing.T) {
	for _, err := range []error{nil, errors.New("older daemon")} {
		store := &clientMetadataStore{err: err}
		server, e := New(store, Config{Actor: "bound", RunID: "run"})
		if e != nil {
			t.Fatal(e)
		}
		result, _, e := server.handle(context.Background(), request{Method: "initialize", Params: json.RawMessage(`{"protocolVersion":"2024-11-05","clientInfo":{"name":"codex-mcp-client","version":"1.2.3","prompt":"SECRET"}}`)})
		if e != nil || result == nil {
			t.Fatalf("metadata failure broke initialization: %v", e)
		}
		if store.actor != "bound" || store.run != "run" || store.client != (bus.AgentClient{Name: "codex-mcp-client", Version: "1.2.3"}) {
			t.Fatalf("wrong report: %+v", store)
		}
	}
}

func TestInitializeWithoutMetadataDoesNotWrite(t *testing.T) {
	store := &clientMetadataStore{}
	server, _ := New(store, Config{Actor: "a", RunID: "r"})
	_, _, err := server.handle(context.Background(), request{Method: "initialize", Params: json.RawMessage(`{}`)})
	if err != nil || store.actor != "" {
		t.Fatalf("empty report: %+v %v", store, err)
	}
}
