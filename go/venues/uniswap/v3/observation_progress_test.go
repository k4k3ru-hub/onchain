package v3

import (
	"context"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"testing"
	"time"
)

// TestIdleObservationSnapshot verifies frozen progress, unchanged provenance and stale-result rejection.
//
// Version:
//   - 2026-10-01: Added.
func TestIdleObservationSnapshot(t *testing.T) {
	c, _ := newTestCache(t)
	if _, err := c.QuotePair(t.Context(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	c.active = true
	c.priceObservationActive = true
	c.running = true
	c.generation = 3
	state, _, ok := c.readPriceObservation()
	if !ok {
		t.Fatal("observation inactive")
	}
	before := c.CaptureQuoteSnapshot().Inputs()
	confirmed := state.ReceivedAt.Add(time.Second)
	if !c.confirmPriceObservation(state, confirmed) {
		t.Fatal("confirmation rejected")
	}
	frozen := c.CaptureQuoteSnapshot()
	got := frozen.Inputs()
	if !got.Observation.ConfirmedAt.Equal(confirmed) || !got.ReceivedAt.Equal(before.ReceivedAt) || got.Position.Sequence != before.Position.Sequence || got.Fields["sqrt_price"] != before.Fields["sqrt_price"] {
		t.Fatal("progress changed input provenance")
	}
	c.generation++
	if c.confirmPriceObservation(state, confirmed.Add(time.Second)) {
		t.Fatal("previous epoch confirmed")
	}
	if !c.CaptureQuoteSnapshot().Inputs().Observation.ConfirmedAt.Equal(before.ReceivedAt) {
		t.Fatal("progress crossed epochs")
	}
	if !frozen.Inputs().Observation.ConfirmedAt.Equal(confirmed) {
		t.Fatal("frozen progress changed")
	}
	state, _, _ = c.readPriceObservation()
	c.running = false
	if c.confirmPriceObservation(state, confirmed.Add(time.Second)) {
		t.Fatal("disconnected state confirmed")
	}
}

type idleWindowRPC struct{ *windowRPC }

// FilterLogs proves the controlled interval contains no pool events.
//
// Version:
//   - 2026-10-01: Added.
func (r *idleWindowRPC) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

// TestRetainedSubscriptionAdvancesIdleObservation verifies automatic progress without publishing a Swap.
//
// Version:
//   - 2026-10-01: Added.
func TestRetainedSubscriptionAdvancesIdleObservation(t *testing.T) {
	c, fake := newTestCache(t)
	rpc := &idleWindowRPC{&windowRPC{fake: fake, height: 100}}
	c.rpc = rpc
	updates := make(chan *QuoteSnapshot, 128)
	c.SetQuoteSnapshotObserver(func(s *QuoteSnapshot) { updates <- s })
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- c.RunRetainedWithEvents(ctx, &stateWS{ready: make(chan struct{})}, big.NewInt(1000000), true, RetainedEvents{})
	}()
	var initial *QuoteSnapshot
	for initial == nil {
		select {
		case s := <-updates:
			if s != nil && s.Inputs().Observation.Active {
				initial = s
			}
		case err := <-done:
			t.Fatal("subscription ended", err)
		case <-ctx.Done():
			t.Fatal("initial capture timed out")
		}
	}
	rpc.mu.Lock()
	rpc.height = 101
	rpc.mu.Unlock()
	for {
		select {
		case s := <-updates:
			if s == nil || !s.Inputs().Observation.ConfirmedAt.After(initial.Inputs().Observation.ConfirmedAt) {
				continue
			}
			if !s.ReceivedAt().Equal(initial.ReceivedAt()) || s.Inputs().Position.Sequence != initial.Inputs().Position.Sequence {
				t.Fatal("idle progress invented a state update")
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if c.CaptureQuoteSnapshot().Inputs().Observation.Active {
				t.Fatal("stopped subscription remained active")
			}
			return
		case err := <-done:
			t.Fatal("subscription ended", err)
		case <-ctx.Done():
			t.Fatal("idle confirmation timed out")
		}
	}
}
