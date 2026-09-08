package clmm

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	sui "github.com/k4k3ru-hub/onchain/go/sui"
	"math/big"
	"testing"
	"time"
)

type stateReaderFake struct {
	t           *testing.T
	obj         *sui.Object
	head        sui.Checkpoint
	reads       int
	cp          sui.CheckpointSequenceNumber
	afterRead   func()
	missingTick bool
	paused      bool
	fail        error
}

// LatestCheckpoint returns the configured indexed head.
//
// Version:
//   - 2026-09-08: Added.
func (f *stateReaderFake) LatestCheckpoint(context.Context) (sui.Checkpoint, error) {
	return f.head, nil
}

// ObjectAtCheckpoint verifies the explicit capture checkpoint.
//
// Version:
//   - 2026-09-08: Added.
func (f *stateReaderFake) ObjectAtCheckpoint(_ context.Context, _ sui.Address, cp sui.CheckpointSequenceNumber) (*sui.Object, error) {
	if cp != f.head.SequenceNumber {
		f.t.Fatal("pool checkpoint mismatch")
	}
	return f.obj, nil
}

// DynamicValuesByKeysAtCheckpoint supplies bitmap words and signed liquidity at the pinned checkpoint.
//
// Version:
//   - 2026-09-08: Added.
func (f *stateReaderFake) DynamicValuesByKeysAtCheckpoint(_ context.Context, parent sui.Address, cp sui.CheckpointSequenceNumber, keys []sui.DynamicFieldKey) ([]json.RawMessage, error) {
	f.reads++
	if f.fail != nil {
		return nil, f.fail
	}
	if cp != f.cp {
		f.t.Fatalf("mixed checkpoint %d want %d", cp, f.cp)
	}
	if f.afterRead != nil {
		f.afterRead()
	}
	if parent == f.obj.Address {
		if f.paused {
			return []json.RawMessage{json.RawMessage("true"), nil}, nil
		}
		return make([]json.RawMessage, len(keys)), nil
	}
	if len(keys) != 1 || keys[0].Type != "0x1::i32::I32" || len(keys[0].BCS) != 4 {
		f.t.Fatal("invalid signed key")
	}
	index := int32(binary.LittleEndian.Uint32(keys[0].BCS))
	n := new(big.Int)
	if parent.String() == testAddress("0xb").String() {
		// Initialized ticks at -100 and +100, with spacing one.
		if index == -1 {
			n.SetBit(n, 156, 1)
		}
		if index == 0 {
			n.SetBit(n, 100, 1)
		}
		return []json.RawMessage{json.RawMessage(fmt.Sprintf("%q", n.String()))}, nil
	}
	if f.missingTick {
		return []json.RawMessage{nil}, nil
	}
	return []json.RawMessage{json.RawMessage(`{"liquidity_net":{"bits":"0"}}`)}, nil
}
func testAddress(s string) sui.Address {
	a, err := sui.ParseAddress(s)
	if err != nil {
		panic(err)
	}
	return a
}
func stateFixture(t *testing.T) (*StateCache, *stateReaderFake, QuotePairParams) {
	t.Helper()
	f := &stateReaderFake{t: t, head: sui.Checkpoint{SequenceNumber: 123, Timestamp: time.Now()}, cp: 123}
	f.obj = &sui.Object{Address: testAddress("0x9"), Version: 1, Move: &sui.MoveObject{Type: "0x1::pool::Pool<0x2::sui::SUI,0x3::usdc::USDC>", JSON: json.RawMessage(`{"reserve_x":"10000000","reserve_y":"10000000","sqrt_price":"18446744073709551616","liquidity":"1000000","tick_index":{"bits":"0"},"swap_fee_rate":"3000","tick_spacing":1,"ticks":{"id":"0xa"},"tick_bitmap":{"id":"0xb"}}`)}}
	c, err := NewStateCache(f, f.obj.Address, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	p := Pool{Address: f.obj.Address}
	params := QuotePairParams{Bid: QuoteExactInputParams{Pool: p, AmountIn: 1000, XForY: true, SqrtPriceLimit: sqrtAtTick(-443636)}, Ask: QuoteExactOutputParams{Pool: p, AmountOut: 1000, XForY: false, SqrtPriceLimit: sqrtAtTick(443636)}}
	return c, f, params
}

// TestCachedPairRoundingAndReuse verifies fee-inclusive input, fresh observations and same-version reuse.
//
// Version:
//   - 2026-09-08: Added.
func TestCachedPairRoundingAndReuse(t *testing.T) {
	c, f, p := stateFixture(t)
	r, err := c.QuotePair(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Bid.AmountIn != 1000 || r.Bid.AmountOut != 996 || r.Bid.FeeAmount != 3 || r.Ask.AmountIn != 1005 || r.Ask.AmountOut != 1000 || r.Ask.FeeAmount != 3 {
		t.Fatalf("pair=%+v %+v", r.Bid, r.Ask)
	}
	count := f.reads
	f.head.SequenceNumber = 124
	f.head.Timestamp = time.Now()
	r, err = c.QuotePair(context.Background(), p)
	if err != nil || f.reads != count || r.Checkpoint != 124 || !r.StateTimestamp.Equal(f.head.Timestamp) {
		t.Fatalf("reuse: reads %d/%d err %v", f.reads, count, err)
	}
	f.obj.Version++
	f.cp = 124
	if _, err = c.QuotePair(context.Background(), p); err != nil || f.reads == count {
		t.Fatalf("refresh: %v", err)
	}
}

// TestCacheRejectsInvalidationAndUnavailableState verifies that failed reads and pool changes cannot publish a quote.
//
// Version:
//   - 2026-09-08: Added.
func TestCacheRejectsInvalidationAndUnavailableState(t *testing.T) {
	for _, kind := range []string{"floor", "during", "expired", "paused", "rpc", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			c, f, p := stateFixture(t)
			ctx := context.Background()
			sentinel := errors.New("unavailable")
			switch kind {
			case "floor":
				c.ObserveCheckpoint(124)
			case "during":
				f.afterRead = func() { c.ObserveCheckpoint(124) }
			case "expired":
				f.head.Timestamp = time.Now().Add(-time.Minute)
			case "paused":
				f.paused = true
			case "rpc":
				f.fail = sentinel
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				c.gate <- struct{}{}
				cancel()
			}
			_, err := c.QuotePair(ctx, p)
			if err == nil {
				t.Fatal("accepted invalid state")
			}
			if kind == "rpc" && !errors.Is(err, sentinel) {
				t.Fatal("lost cause")
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation")
			}
		})
	}
}

// TestBitmapBoundariesAndCrossing verifies signed compression, empty-word boundaries and missing initialized ticks.
//
// Version:
//   - 2026-09-08: Added.
func TestBitmapBoundariesAndCrossing(t *testing.T) {
	c, f, _ := stateFixture(t)
	s, err := capturePool(f.obj, 123)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		tick        int32
		down        bool
		want        int32
		initialized bool
	}{{0, true, 0, false}, {-1, true, -100, true}, {0, false, 100, true}, {100, false, 255, false}, {-257, true, -512, false}} {
		n, ok, e := c.nextTick(context.Background(), s, tt.tick, tt.down)
		if e != nil || n != tt.want || ok != tt.initialized {
			t.Fatalf("%+v: %d %v %v", tt, n, ok, e)
		}
	}
	// A larger swap crosses initialized ticks and empty bitmap words.
	r, err := c.quote(context.Background(), s, 20000, true, true, sqrtAtTick(-1000))
	if err != nil || r.AmountOut == 0 {
		t.Fatalf("crossing: %+v %v", r, err)
	}
	f.missingTick = true
	s.nets = map[int32]*big.Int{}
	if _, err = c.quote(context.Background(), s, 20000, true, true, sqrtAtTick(-1000)); err == nil {
		t.Fatal("accepted missing initialized tick")
	}
	if _, err = c.quote(context.Background(), s, 20000, false, true, sqrtAtTick(1)); err == nil {
		t.Fatal("accepted partial fill")
	}
}

// TestTickProtocolBounds verifies exact Q64 protocol constants.
//
// Version:
//   - 2026-09-08: Added.
func TestTickProtocolBounds(t *testing.T) {
	for tick, want := range map[int32]string{-443636: "4295048016", 0: "18446744073709551616", 443636: "79226673515401279992447579055", -1: "18445821805675392311"} {
		if got := sqrtAtTick(tick).String(); got != want {
			t.Fatalf("tick %d: %s", tick, got)
		}
	}
}

// TestQuoteDirectionsAndModes checks both exact-input and exact-output paths against manual Q64 rounding.
//
// Version:
//   - 2026-09-08: Added.
func TestQuoteDirectionsAndModes(t *testing.T) {
	for _, down := range []bool{true, false} {
		for _, exact := range []bool{true, false} {
			c, f, _ := stateFixture(t)
			state, err := capturePool(f.obj, 123)
			if err != nil {
				t.Fatal(err)
			}
			limit := sqrtAtTick(1000)
			if down {
				limit = sqrtAtTick(-1000)
			}
			r, err := c.quote(context.Background(), state, 1000, down, exact, limit)
			if err != nil {
				t.Fatal(err)
			}
			in, out, fee := uint64(1000), uint64(996), uint64(3)
			if !exact {
				in, out, fee = 1005, 1000, 3
			}
			if r.AmountIn != in || r.AmountOut != out || r.FeeAmount != fee {
				t.Fatalf("down %v exact %v: %+v", down, exact, r)
			}
		}
	}
}

// TestMomentumFeeRounding guards the deployed nearest-integer fee rule against a ceiling regression.
//
// Version:
//   - 2026-09-08: Added.
func TestMomentumFeeRounding(t *testing.T) {
	if got := localFee(big.NewInt(8269), 1750).Uint64(); got != 14 {
		t.Fatalf("fee %d want 14", got)
	}
	if got := localFee(big.NewInt(828724), 1750).Uint64(); got != 1453 {
		t.Fatalf("fee %d want 1453", got)
	}
}
