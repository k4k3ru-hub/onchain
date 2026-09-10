package spot

import (
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"reflect"
	"testing"
)

// TestQuoteInputsAreDetached verifies retained input metadata and ordering.
//
// Version:
//   - 2026-09-10: Added.
func TestQuoteInputsAreDetached(t *testing.T) {
	c, _, p := stateFixture(t)
	if _, err := c.QuotePair(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	index := uint64(0)
	c.retained.position = &quotestate.Position{Kind: "transaction", Sequence: 101, Digest: "transaction", Index: &index}
	frozen := c.CaptureQuoteSnapshot()
	want := frozen.Inputs()
	got := frozen.Inputs()
	if got.Position.Kind != "transaction" || got.Position.Sequence != 101 || got.Position.Index == nil || *got.Position.Index != 0 || got.Baseline.Kind != "checkpoint" {
		t.Fatalf("lost live position: %+v", got)
	}
	if got.ReceivedAt.IsZero() || got.Fields["sqrt_price"] == "" {
		t.Fatal("missing frozen metadata")
	}
	got.Fields["sqrt_price"] = "changed"
	*got.Position.Index = 99
	if len(got.Coverage) > 0 {
		got.Coverage[0].Lower = 999
	}
	c.retained = nil
	if !reflect.DeepEqual(want, frozen.Inputs()) {
		t.Fatal("mutable metadata or live cache alias")
	}
	var empty *QuoteSnapshot
	if empty.Inputs() != nil {
		t.Fatal("nil snapshot has inputs")
	}
}
