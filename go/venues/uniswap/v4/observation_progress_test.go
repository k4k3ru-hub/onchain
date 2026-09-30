package v4

import (
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
