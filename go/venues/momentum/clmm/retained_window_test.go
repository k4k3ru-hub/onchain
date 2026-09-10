package clmm

import (
	"context"
	"errors"
	"github.com/k4k3ru-hub/onchain/go/internal/suiwindow"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"testing"
	"time"
)

// TestWindowPreloadsUnvisitedTicks verifies a later crossing quotes without fetching tick details.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowPreloadsUnvisitedTicks(t *testing.T) {
	c, reader, params := stateFixture(t)
	ctx := context.Background()
	if _, err := c.QuotePair(ctx, params); err != nil {
		t.Fatal(err)
	}
	if len(c.retained.words) != 3 || len(c.retained.nets) != 2 {
		t.Fatal("nearby tick details were not acquired")
	}
	calls := reader.reads
	reader.fail = errors.New("network must not be used")
	c.retained.pool.TickIndex = 90
	c.retained.pool.SqrtPrice = sqrtAtTick(90)
	if _, err := c.QuoteRetainedPair(ctx, params); err != nil {
		t.Fatal("future tick crossing failed", err)
	}
	if reader.reads != calls {
		t.Fatal("live quote used RPC")
	}
	if c.retainedCoverageMissing(ctx) {
		t.Fatal("complete window reported missing")
	}
	delete(c.retained.nets, 100)
	if !c.retainedCoverageMissing(ctx) {
		t.Fatal("missing tick was not detected")
	}
}

// TestWindowInstallationReplaysAndRejectsFailure verifies refresh never rolls back or withdraws live state.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowInstallationReplaysAndRejectsFailure(t *testing.T) {
	c, reader, params := stateFixture(t)
	ctx := context.Background()
	if _, err := c.QuotePair(ctx, params); err != nil {
		t.Fatal(err)
	}
	candidate, err := c.captureRetainedWindow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after := *reader.obj
	after.Version++
	cp := sui.CheckpointSequenceNumber(124)
	n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp, Successful: true}, ObjectChanges: []sui.ObjectChange{{Address: c.pool, After: &after}}}
	received := time.Now().UTC()
	if err := c.applyRetainedObjects(n, received); err != nil {
		t.Fatal(err)
	}
	live := c.retained
	if err := c.installRetainedWindow(candidate, nil); err == nil || c.retained != live {
		t.Fatal("regressed capture replaced live state")
	}
	if err := c.installRetainedWindow(candidate, []suiwindow.Update{{Notification: n, ReceivedAt: received}}); err != nil {
		t.Fatal(err)
	}
	if c.retained.version != after.Version || !c.retained.received.Equal(received) {
		t.Fatal("concurrent update or receipt lost")
	}
	live = c.retained
	broken := &StateCache{pool: c.pool, retained: cloneRetainedState(live), retainedHead: c.retainedHead}
	malformed := *n
	malformed.ObjectChanges = []sui.ObjectChange{{Address: c.pool, After: &sui.Object{Address: c.pool, Version: after.Version + 1}}}
	if err := c.installRetainedWindow(broken, []suiwindow.Update{{Notification: &malformed, ReceivedAt: received}}); err == nil || c.retained != live {
		t.Fatal("failed replay modified live state")
	}
}
