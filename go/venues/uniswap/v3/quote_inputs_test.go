package v3

import (
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"reflect"
	"testing"
)

// TestQuoteInputsAreDetached verifies retained input metadata and ordering.
//
// Version:
//   - 2026-09-10: Added.
func TestQuoteInputsAreDetached(t *testing.T) {
	c, _ := newTestCache(t)
	if _, err := c.QuotePair(t.Context(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	c.retainedBlock, c.retainedHash, c.retainedIndex, c.retainedHasLog = 101, common.HexToHash("02"), 0, true
	c.retained.words = map[int32]*big.Int{-2: big.NewInt(0), -1: big.NewInt(0), 2: big.NewInt(0)}
	frozen := c.CaptureQuoteSnapshot()
	want := frozen.Inputs()
	got := frozen.Inputs()
	if got.Position.Kind != "log" || got.Position.Sequence != 101 || got.Position.Index == nil || *got.Position.Index != 0 || got.Baseline.Kind != "block" {
		t.Fatalf("lost live position: %+v", got)
	}
	if len(got.Coverage) != 2 || got.Coverage[0].Lower != -2 || got.Coverage[0].Upper != -1 || got.Coverage[1].Lower != 2 {
		t.Fatal("invented contiguous coverage", got.Coverage)
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
