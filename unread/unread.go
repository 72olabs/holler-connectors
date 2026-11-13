// Package unread counts what is waiting for the bound actor in both delivery
// generations: the legacy inbox (bus_* tools) and managed conversation
// deliveries (holler_channel_* tools, direct chats and rooms). SessionStart and
// bus_status share it so the two never disagree about what "unread" means.
//
// It only lists. Nothing here claims, acknowledges or marks a delivery read.
//
// It is supported public API: the Holler daemon imports it too.
package unread

import (
	"context"
	"encoding/json"
	"time"

	"github.com/72olabs/holler-connectors/bus"
)

// Limit is the page size used for each inbox. A full legacy page means the
// legacy count is a lower bound; see Counts.ManagedLowerBound for managed.
const Limit = 100

// ManagedInboxCapability is the daemon read capability behind
// holler_channel_inbox.
const ManagedInboxCapability = "channel.inbox"

// LegacyLister is the legacy inbox listing used by SessionStart and bus_status.
type LegacyLister interface {
	CheckInbox(context.Context, string, int) ([]bus.InboxItem, error)
}

// CapabilityReader is the read side of the capability bridge. A store without
// it, or a daemon whose catalog lacks channel.inbox, leaves Managed unchecked.
type CapabilityReader interface {
	ListCapabilities(context.Context) ([]bus.CapabilityDescriptor, error)
	InvokeReadCapability(context.Context, string, json.RawMessage) (json.RawMessage, error)
}

type Counts struct {
	Legacy           int
	LegacyAvailable  int
	LegacyTruncated  bool
	ManagedChecked   bool
	Managed          int
	ManagedAvailable int
	// ManagedLowerBound is true when Managed may undercount. channel.inbox
	// stops at Limit rows and also, on the daemon, once the bodies it has
	// gathered pass a size budget (1.5 times bus.MaxBodyBytes), and its result
	// carries no completeness marker. The budget is measured on stored bodies,
	// which the wire copy may not reproduce byte for byte, so only an empty
	// listing is known to be complete.
	ManagedLowerBound bool
}

// Total is every unread delivery that was counted. When ManagedChecked is
// false it covers the legacy inbox only.
func (c Counts) Total() int { return c.Legacy + c.Managed }

// Count lists both inboxes. A legacy failure is returned as an error, as it
// was before managed deliveries were counted. A managed failure is not: it
// leaves ManagedChecked false so callers can say what they could not check.
func Count(ctx context.Context, store LegacyLister, actor string) (Counts, error) {
	var caps []bus.CapabilityDescriptor
	catalogRead := false
	if reader, ok := store.(CapabilityReader); ok {
		var err error
		caps, err = reader.ListCapabilities(ctx)
		catalogRead = err == nil
	}
	return CountWithCatalog(ctx, store, actor, caps, catalogRead)
}

// CountWithCatalog is Count for a caller that has already read the capability
// catalog. catalogRead is false when the catalog could not be read, which
// leaves managed deliveries unchecked.
func CountWithCatalog(ctx context.Context, store LegacyLister, actor string, caps []bus.CapabilityDescriptor, catalogRead bool) (Counts, error) {
	items, err := store.CheckInbox(ctx, actor, Limit)
	if err != nil {
		return Counts{}, err
	}
	counts := Counts{Legacy: len(items), LegacyTruncated: len(items) >= Limit}
	for _, item := range items {
		if item.Available {
			counts.LegacyAvailable++
		}
	}
	reader, ok := store.(CapabilityReader)
	if !ok || !catalogRead || !HasRead(caps, ManagedInboxCapability) {
		return counts, nil
	}
	return countManaged(ctx, reader, counts), nil
}

func countManaged(ctx context.Context, reader CapabilityReader, counts Counts) Counts {
	raw, err := reader.InvokeReadCapability(ctx, ManagedInboxCapability, json.RawMessage(`{"limit":100}`))
	if err != nil {
		return counts
	}
	// channel.inbox returns the deliveries the daemon still owes this actor:
	// queued ones, and claimed ones whose lease is live (possibly held by
	// another run of the same actor). Only the reference fields are decoded.
	var deliveries []struct {
		State          string     `json:"state"`
		LeaseExpiresAt *time.Time `json:"lease_expires_at"`
	}
	if err := json.Unmarshal(raw, &deliveries); err != nil {
		return counts
	}
	now := time.Now()
	counts.ManagedChecked = true
	counts.Managed = len(deliveries)
	counts.ManagedLowerBound = len(deliveries) > 0
	for _, d := range deliveries {
		if d.State != "claimed" || d.LeaseExpiresAt == nil || !d.LeaseExpiresAt.After(now) {
			counts.ManagedAvailable++
		}
	}
	return counts
}

// HasRead reports whether the catalog offers name as a read capability.
func HasRead(caps []bus.CapabilityDescriptor, name string) bool {
	for _, c := range caps {
		if c.Name == name && c.Mode == bus.CapabilityRead {
			return true
		}
	}
	return false
}
