package unread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/72olabs/holler-connectors/bus"
)

type legacyOnly struct {
	items []bus.InboxItem
	err   error
}

func (s *legacyOnly) CheckInbox(_ context.Context, _ string, limit int) ([]bus.InboxItem, error) {
	if limit != Limit {
		panic(limit)
	}
	return s.items, s.err
}

type bridged struct {
	legacyOnly
	caps    []bus.CapabilityDescriptor
	capsErr error
	raw     string
	readErr error
	reads   []string
}

func (s *bridged) ListCapabilities(context.Context) ([]bus.CapabilityDescriptor, error) {
	return s.caps, s.capsErr
}
func (s *bridged) InvokeReadCapability(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	s.reads = append(s.reads, name)
	return json.RawMessage(s.raw), s.readErr
}

var inboxCatalog = []bus.CapabilityDescriptor{{Name: ManagedInboxCapability, Mode: bus.CapabilityRead}}

func TestCountReadsBothInboxesWithoutConsuming(t *testing.T) {
	live := time.Now().Add(time.Minute).Format(time.RFC3339Nano)
	lapsed := time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
	store := &bridged{
		legacyOnly: legacyOnly{items: []bus.InboxItem{{Available: true}, {Available: false}}},
		caps:       inboxCatalog,
		raw:        `[{"state":"queued"},{"state":"claimed","lease_expires_at":"` + live + `"},{"state":"claimed","lease_expires_at":"` + lapsed + `"}]`,
	}
	got, err := Count(context.Background(), store, "a")
	if err != nil {
		t.Fatal(err)
	}
	want := Counts{Legacy: 2, LegacyAvailable: 1, ManagedChecked: true, Managed: 3, ManagedAvailable: 2, ManagedLowerBound: true}
	if got != want || got.Total() != 5 {
		t.Fatalf("got %+v want %+v", got, want)
	}
	if len(store.reads) != 1 || store.reads[0] != ManagedInboxCapability {
		t.Fatalf("reads %v", store.reads)
	}
}

func TestCountLeavesManagedUncheckedWhenItCannotLook(t *testing.T) {
	for name, store := range map[string]LegacyLister{
		"no bridge":          &legacyOnly{},
		"catalog error":      &bridged{capsErr: errors.New("unknown operation")},
		"not in catalog":     &bridged{caps: []bus.CapabilityDescriptor{{Name: "channel.list", Mode: bus.CapabilityRead}}},
		"write-mode entry":   &bridged{caps: []bus.CapabilityDescriptor{{Name: ManagedInboxCapability, Mode: bus.CapabilityWrite}}},
		"read error":         &bridged{caps: inboxCatalog, readErr: errors.New("disabled")},
		"unexpected payload": &bridged{caps: inboxCatalog, raw: `{"result":"not a list"}`},
	} {
		got, err := Count(context.Background(), store, "a")
		if err != nil || got.ManagedChecked || got.Managed != 0 {
			t.Fatalf("%s: %+v %v", name, got, err)
		}
	}
}

func TestCountReturnsLegacyErrorAndMarksFullPages(t *testing.T) {
	if _, err := Count(context.Background(), &legacyOnly{err: errors.New("down")}, "a"); err == nil {
		t.Fatal("legacy failure was hidden")
	}
	items := make([]bus.InboxItem, Limit)
	raw := "["
	for i := 0; i < Limit; i++ {
		raw += fmt.Sprintf(`{"state":"queued","n":%d},`, i)
	}
	raw = raw[:len(raw)-1] + "]"
	got, err := Count(context.Background(), &bridged{legacyOnly: legacyOnly{items: items}, caps: inboxCatalog, raw: raw}, "a")
	if err != nil || !got.LegacyTruncated || !got.ManagedLowerBound || got.Total() != 2*Limit {
		t.Fatalf("%+v %v", got, err)
	}
}

// The daemon also ends channel.inbox at a body-size budget without saying so:
// with two queued messages whose bodies are near bus.MaxBodyBytes it returns
// one. A short page is therefore a lower bound; only an empty one is complete.
func TestManagedCountIsALowerBoundUnlessEmpty(t *testing.T) {
	body, _ := json.Marshal(strings.Repeat("x", bus.MaxBodyBytes-16))
	onePage := `[{"state":"queued","message":{"message_id":"m1","body":` + string(body) + `}}]`
	got, err := Count(context.Background(), &bridged{caps: inboxCatalog, raw: onePage}, "a")
	if err != nil || got.Managed != 1 || !got.ManagedLowerBound {
		t.Fatalf("size-cut page reported as complete: %+v %v", got, err)
	}
	got, err = Count(context.Background(), &bridged{caps: inboxCatalog, raw: `[]`}, "a")
	if err != nil || !got.ManagedChecked || got.Managed != 0 || got.ManagedLowerBound {
		t.Fatalf("empty listing: %+v %v", got, err)
	}
}
