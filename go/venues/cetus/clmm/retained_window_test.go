package clmm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/k4k3ru-hub/onchain/go/internal/suiwindow"
	"github.com/k4k3ru-hub/onchain/go/sui"
)

type windowReaderFake struct {
	*stateReaderFake
	keys    []uint64
	calls   int
	failAt  int
	onBatch func()
	ticks   map[int32]json.RawMessage
}

// DynamicUint64ValuesAtCheckpoint serves only explicitly indexed test nodes.
//
// Version:
//   - 2026-09-11: Added.
func (f *windowReaderFake) DynamicUint64ValuesAtCheckpoint(ctx context.Context, _ sui.Address, cp sui.CheckpointSequenceNumber, keys []uint64) ([]json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cp != f.cp || len(keys) > 50 {
		return nil, fmt.Errorf("failed to read test ticks: request=invalid")
	}
	f.calls++
	f.keys = append(f.keys, keys...)
	if f.onBatch != nil {
		f.onBatch()
	}
	if f.calls == f.failAt {
		return nil, fmt.Errorf("failed to read test ticks: response=unavailable")
	}
	values := make([]json.RawMessage, len(keys))
	for i, key := range keys {
		values[i] = f.ticks[int32(int64(key)-443636)]
	}
	return values, nil
}

func windowFixture(t *testing.T) (*StateCache, *windowReaderFake, QuotePairParams) {
	c, f, p := cacheFixture(t)
	f.obj.Move.JSON = json.RawMessage(strings.Replace(string(f.obj.Move.JSON), `"tick_spacing":"100"`, `"tick_spacing":"1"`, 1))
	r := &windowReaderFake{stateReaderFake: f, ticks: map[int32]json.RawMessage{}}
	c.reader = r
	return c, r, p
}

func windowPool(t *testing.T, source *sui.Object, tick int32, version uint64) *sui.Object {
	t.Helper()
	obj := *source
	move := *source.Move
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(move.JSON, &fields); err != nil {
		t.Fatal(err)
	}
	fields["current_tick_index"] = json.RawMessage(fmt.Sprintf(`{"bits":%d}`, uint32(tick)))
	fields["current_sqrt_price"] = json.RawMessage(fmt.Sprintf(`"%s"`, sqrtAtTick(tick)))
	var err error
	move.JSON, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	obj.Move, obj.Version = &move, version
	return &obj
}

// TestWindowAcquisitionIsBounded verifies large pools never trigger global scans and future ticks are loaded.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowAcquisitionIsBounded(t *testing.T) {
	for _, count := range []string{"2", "1000000"} {
		c, r, p := windowFixture(t)
		r.obj.Move.JSON = json.RawMessage(strings.Replace(string(r.obj.Move.JSON), `"size":"2"`, `"size":"`+count+`"`, 1))
		r.ticks[400] = json.RawMessage(fmt.Sprintf(`{"value":{"index":{"bits":400},"sqrt_price":"%s","liquidity_net":{"bits":"0"}}}`, sqrtAtTick(400)))
		if err := c.Warm(context.Background()); err != nil {
			t.Fatal(err)
		}
		if r.pages != 0 || r.calls != 16 || len(r.keys) != 768 || len(c.retained.snapshot.Ticks) != 1 {
			t.Fatalf("scan/batch coverage mismatch: pages=%d batches=%d keys=%d", r.pages, r.calls, len(r.keys))
		}
		seen := map[uint64]bool{}
		for _, key := range r.keys {
			if key < 443380 || key > 444147 || seen[key] {
				t.Fatal("unbounded or duplicate key", key)
			}
			seen[key] = true
		}
		if _, err := c.QuoteRetainedPair(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		if r.calls != 16 {
			t.Fatal("local quote issued RPC")
		}
		inputs := c.CaptureQuoteSnapshot().Inputs()
		if len(inputs.Coverage) != 1 || inputs.Coverage[0].Lower != -256 || inputs.Coverage[0].Upper != 511 {
			t.Fatal("incorrect coverage", inputs.Coverage)
		}
	}
}

// TestWindowEmptyIntervalsAndBoundaries verifies sparse local quotes, bounded rejection and signed alignment.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowEmptyIntervalsAndBoundaries(t *testing.T) {
	c, r, p := windowFixture(t)
	if err := c.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, a2b := range []bool{true, false} {
		if _, err := c.retained.snapshot.Quote(1000, a2b, true); err != nil {
			t.Fatal(err)
		}
		if _, err := c.retained.snapshot.Quote(100000000, a2b, true); !errors.Is(err, suiwindow.ErrCoverage) {
			t.Fatal("missing coverage accepted", err)
		}
	}
	if _, err := c.QuoteRetainedPair(context.Background(), p); err != nil || r.calls != 16 {
		t.Fatal("local quote failure", err)
	}
	for _, tick := range []int32{-443636, -257, -1, 0, 255, 256, 443636} {
		w, err := newTickWindow(tick, 10)
		if err != nil {
			t.Fatal(err)
		}
		if w.first < -443636 || w.last > 443636 || w.first%10 != 0 || w.last%10 != 0 || (w.last-w.first)/10+1 > 768 || tick < w.lower || tick > w.upper {
			t.Fatalf("invalid bounds: %+v", w)
		}
	}
	w, err := newTickWindow(-1, 1)
	if err != nil || w.center != -1 || w.first != -512 || w.last != 255 {
		t.Fatalf("negative flooring: %+v %v", w, err)
	}
	for _, v := range []struct {
		tick  int32
		price string
	}{{-443636, "4295048016"}, {0, "18446744073709551616"}, {443636, "79226673515401279992447579055"}} {
		if sqrtAtTick(v.tick).String() != v.price {
			t.Fatal("tick math vector mismatch", v.tick)
		}
	}
}

// TestWindowCaptureFailurePreservesLiveState verifies failed batches and cancellation never install partial ranges.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowCaptureFailurePreservesLiveState(t *testing.T) {
	c, r, _ := windowFixture(t)
	if err := c.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	old := c.retained
	r.failAt = r.calls + 2
	if _, err := c.captureRetainedWindow(context.Background()); err == nil || c.retained != old {
		t.Fatal("partial candidate installed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.onBatch = cancel
	if _, err := c.captureRetainedWindow(ctx); !errors.Is(err, context.Canceled) || c.retained != old {
		t.Fatal("cancelled candidate installed", err)
	}
	if r.pages != 0 {
		t.Fatal("fell back to full scan")
	}
}

// TestWindowRefillReplaysLiveUpdates verifies recentering and preservation of newer live state and frozen inputs.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowRefillReplaysLiveUpdates(t *testing.T) {
	c, r, _ := windowFixture(t)
	ctx := context.Background()
	if err := c.Warm(ctx); err != nil {
		t.Fatal(err)
	}
	frozen := c.CaptureQuoteSnapshot()
	cp := sui.CheckpointSequenceNumber(124)
	after := windowPool(t, r.obj, 260, 11)
	n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp}, ObjectChanges: []sui.ObjectChange{{Address: c.pool, After: after}}}
	if err := c.applyRetainedObjects(n); err != nil {
		t.Fatal(err)
	}
	if !c.retainedCoverageMissing() {
		t.Fatal("missing recenter signal")
	}
	r.obj, r.cp = after, cp
	cp2 := sui.CheckpointSequenceNumber(125)
	newer := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp2}, ObjectChanges: []sui.ObjectChange{{Address: c.pool, After: windowPool(t, after, 270, 12)}}}
	at := time.Now().UTC().Add(time.Second)
	r.onBatch = func() {
		r.onBatch = nil
		if err := c.applyRetainedObjects(newer, at); err != nil {
			t.Fatal(err)
		}
	}
	candidate, err := c.captureRetainedWindow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c.retained.version != 12 {
		t.Fatal("live application blocked/lost")
	}
	if err := c.installRetainedWindow(candidate, []suiwindow.Update{{Notification: newer, ReceivedAt: at}}); err != nil {
		t.Fatal(err)
	}
	if c.retained.version != 12 || c.retained.snapshot.Pool.CurrentTickIndex != 270 || !c.retained.received.Equal(at) || c.retainedCoverageMissing() {
		t.Fatal("candidate lost live state")
	}
	if frozen.cache.retained.snapshot.window.center != 0 || frozen.cache.retained.snapshot.Pool.CurrentTickIndex != 0 {
		t.Fatal("frozen inputs mutated")
	}
	old := c.retained
	if err := c.installRetainedWindow(&frozen.cache, nil); err == nil || c.retained != old {
		t.Fatal("regression accepted")
	}
}

// TestWindowIgnoresOutsideTickDeletion verifies unrelated ticks do not invalidate bounded retained state.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowIgnoresOutsideTickDeletion(t *testing.T) {
	c, r, _ := windowFixture(t)
	if err := c.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	cp := sui.CheckpointSequenceNumber(124)
	tick := &sui.Object{Move: &sui.MoveObject{JSON: json.RawMessage(`{"value":{"value":{"index":{"bits":10000},"sqrt_price":"2","liquidity_net":{"bits":"0"}}}}`)}}
	n := &sui.TransactionNotification{Effects: &sui.TransactionEffects{Checkpoint: &cp}, ObjectChanges: []sui.ObjectChange{{Address: c.pool, After: windowPool(t, r.obj, 0, 11)}, {InputParent: c.retained.handle, Before: tick, Deleted: true}}}
	if err := c.applyRetainedObjects(n); err != nil || c.retained == nil {
		t.Fatal("outside deletion invalidated state", err)
	}
	if len(c.retained.snapshot.Ticks) != 0 || c.retained.snapshot.Pool.CurrentSqrtPrice.Cmp(new(big.Int).Lsh(big.NewInt(1), 64)) != 0 {
		t.Fatal("unexpected state mutation")
	}
}

// TestWindowFutureTickQuotesLocally verifies preloaded ticks support later larger quotes without supplementation.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowFutureTickQuotesLocally(t *testing.T) {
	c, r, p := windowFixture(t)
	r.ticks[400] = json.RawMessage(fmt.Sprintf(`{"value":{"index":{"bits":400},"sqrt_price":"%s","liquidity_net":{"bits":"500000"}}}`, sqrtAtTick(400)))
	if err := c.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.QuoteRetainedPair(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	full := *cloneLocalSnapshot(c.retained.snapshot)
	full.window = nil
	full.Ticks = append(full.Ticks, Tick{Index: 1000, SqrtPrice: sqrtAtTick(1000), LiquidityNet: new(big.Int)})
	actual, err := c.retained.snapshot.Quote(23000, false, true)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := full.Quote(23000, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if actual.AfterSqrtPrice.Cmp(sqrtAtTick(400)) <= 0 || actual.AfterSqrtPrice.Cmp(expected.AfterSqrtPrice) != 0 || actual.AmountOut != expected.AmountOut || r.calls != 16 {
		t.Fatal("future tick was not used locally")
	}
}

type legacyWindowReader struct{ StateReader }

// TestWindowDoesNotFallbackToEnumeration verifies readers without keyed access fail without scanning the pool.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowDoesNotFallbackToEnumeration(t *testing.T) {
	c, r, _ := windowFixture(t)
	c.reader = legacyWindowReader{r}
	if err := c.Warm(context.Background()); err == nil || !strings.Contains(err.Error(), "keyed_reader=unsupported") {
		t.Fatal("unsupported reader accepted", err)
	}
	if r.pages != 0 || r.calls != 0 || c.retained != nil {
		t.Fatal("full-scan fallback was used")
	}
}
