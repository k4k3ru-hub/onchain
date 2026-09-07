package slipstream

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type stateFake struct {
	t          *testing.T
	calls      int
	reorg      bool
	crossed    bool
	hook       func()
	blocks     []uint64
	fee        *big.Int
	feeError   error
	stakingNet *big.Int
}

// LatestHeader implements the injected test dependency.
//
// Version:
//   - 2026-09-08: Added.
func (f *stateFake) LatestHeader(context.Context) (evm.BlockHeader, error) {
	return evm.BlockHeader{Number: 100, Hash: common.HexToHash("01")}, nil
}

// HeaderByNumber implements the injected test dependency.
//
// Version:
//   - 2026-09-08: Added.
func (f *stateFake) HeaderByNumber(context.Context, uint64) (evm.BlockHeader, error) {
	h := common.HexToHash("01")
	if f.reorg {
		h = common.HexToHash("02")
	}
	return evm.BlockHeader{Number: 100, Hash: h}, nil
}

// CallContract implements the injected test dependency.
//
// Version:
//   - 2026-09-08: Added.
func (f *stateFake) CallContract(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	f.calls++
	if block == nil {
		return nil, fmt.Errorf("unpinned read")
	}
	f.blocks = append(f.blocks, block.Uint64())
	if f.hook != nil {
		f.hook()
	}
	sig := string(msg.Data[:4])
	selector := func(s string) string { return string(crypto.Keccak256([]byte(s))[:4]) }
	switch sig {
	case selector("slot0()"):
		args := abi.Arguments{{Type: mustABIType(f.t, "uint160")}, {Type: mustABIType(f.t, "int24")}, {Type: mustABIType(f.t, "uint16")}, {Type: mustABIType(f.t, "uint16")}, {Type: mustABIType(f.t, "uint16")}, {Type: mustABIType(f.t, "bool")}}
		return args.Pack(power2(96), big.NewInt(0), uint16(0), uint16(0), uint16(0), true)
	case selector("fee()"):
		if f.feeError != nil {
			return nil, f.feeError
		}
		if f.fee != nil {
			return f.fee.FillBytes(make([]byte, 32)), nil
		}
		return big.NewInt(3000).FillBytes(make([]byte, 32)), nil
	case selector("liquidity()"):
		return integer("1000000000000000000").FillBytes(make([]byte, 32)), nil
	case selector("tickSpacing()"):
		return big.NewInt(60).FillBytes(make([]byte, 32)), nil
	case selector("tickBitmap(int16)"):
		n := new(big.Int)
		if f.crossed {
			if msg.Data[4] == 255 {
				n.SetBit(n, 255, 1)
			} else {
				n.SetBit(n, 1, 1)
			}
		}
		return n.FillBytes(make([]byte, 32)), nil
	case selector("ticks(int24)"):
		data := make([]byte, 320)
		big.NewInt(1).FillBytes(data[:32])
		integer("100000000000000000").FillBytes(data[32:64])
		data[319] = 1
		if f.stakingNet != nil {
			f.stakingNet.FillBytes(data[64:96])
		}
		return data, nil
	}
	return nil, fmt.Errorf("unexpected selector")
}
func newTestCache(t *testing.T) (*StateCache, *stateFake) {
	t.Helper()
	f := &stateFake{t: t}
	c, err := NewStateCache(f, common.HexToAddress("01"), common.HexToAddress("02"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

// TestCacheReuseExpiryAndDetachedAmounts verifies cache reuse expiry and detached amounts.
//
// Version:
//   - 2026-09-08: Added.
func TestCacheReuseExpiryAndDetachedAmounts(t *testing.T) {
	c, f := newTestCache(t)
	c.active = true
	a, err := c.QuotePair(context.Background(), big.NewInt(1000000), true)
	if err != nil {
		t.Fatal(err)
	}
	calls := f.calls
	a.BidAmountOut.SetInt64(0)
	b, err := c.QuotePair(context.Background(), big.NewInt(1000000), true)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != calls || b.BidAmountOut.Sign() <= 0 || !a.ObservedAt.Equal(b.ObservedAt) {
		t.Fatal("cache reuse failed")
	}
	for _, block := range f.blocks {
		if block != 100 {
			t.Fatal("mixed blocks")
		}
	}
	c.snapshot.observed = time.Now().Add(-2 * time.Minute)
	if _, err = c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	if f.calls <= calls {
		t.Fatal("expired snapshot reused")
	}
	c.active = false
	calls = f.calls
	if _, err = c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	if f.calls <= calls {
		t.Fatal("unwatched snapshot reused")
	}
}

// TestCacheRejectsReorgAndConcurrentInvalidation verifies cache rejects reorg and concurrent invalidation.
//
// Version:
//   - 2026-09-08: Added.
func TestCacheRejectsReorgAndConcurrentInvalidation(t *testing.T) {
	for _, reorg := range []bool{true, false} {
		c, f := newTestCache(t)
		c.active = true
		f.reorg = reorg
		if !reorg {
			f.hook = func() { c.mu.Lock(); c.generation++; c.mu.Unlock() }
		}
		if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err == nil {
			t.Fatal("accepted inconsistent snapshot")
		}
	}
}

// TestCacheCrossesInitializedTicks verifies cache crosses initialized ticks.
//
// Version:
//   - 2026-09-08: Added.
func TestCacheCrossesInitializedTicks(t *testing.T) {
	for _, zero := range []bool{true, false} {
		c, f := newTestCache(t)
		f.crossed = true
		got, err := c.QuotePair(context.Background(), integer("10000000000000000"), zero)
		if err != nil {
			t.Fatal(err)
		}
		if got.BidAmountOut.Sign() <= 0 || got.AskAmountIn.Cmp(integer("10000000000000000")) <= 0 || len(c.snapshot.ticks) == 0 {
			t.Fatal("tick crossing not exercised")
		}
	}
}

// TestCacheConcurrentReuseAndCancellation verifies cache concurrent reuse and cancellation.
//
// Version:
//   - 2026-09-08: Added.
func TestCacheConcurrentReuseAndCancellation(t *testing.T) {
	c, f := newTestCache(t)
	c.active = true
	if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	calls := f.calls
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if f.calls != calls {
		t.Fatal("concurrent cache misses")
	}
	<-c.gate
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.QuotePair(ctx, big.NewInt(1), true); err == nil {
		t.Fatal("cancellation ignored")
	}
	c.gate <- struct{}{}
}

type stateWS struct {
	logs  chan<- types.Log
	ready chan struct{}
}

// SubscribeFilterLogs implements the injected test dependency.
//
// Version:
//   - 2026-09-08: Added.
func (w *stateWS) SubscribeFilterLogs(_ context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	if len(q.Topics) != 0 || len(q.Addresses) != 2 {
		return nil, fmt.Errorf("state events filtered")
	}
	w.logs = ch
	close(w.ready)
	return event.NewSubscription(func(quit <-chan struct{}) error { <-quit; return nil }), nil
}

// TestStateSubscriptionInvalidatesAllLogsAndDisconnect verifies state subscription invalidates all logs and disconnect.
//
// Version:
//   - 2026-09-08: Added.
func TestStateSubscriptionInvalidatesAllLogsAndDisconnect(t *testing.T) {
	c, _ := newTestCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ws := &stateWS{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, ws) }()
	<-ws.ready
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		active := c.active
		c.mu.Unlock()
		if active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not active")
		}
		time.Sleep(time.Millisecond)
	}
	if err := c.Run(ctx, ws); err == nil {
		t.Fatal("concurrent subscription accepted")
	}
	ws.logs <- types.Log{Address: c.pool, BlockNumber: 101, Removed: true}
	for {
		c.mu.Lock()
		floor := c.floor
		c.mu.Unlock()
		if floor == 101 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not invalidated")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active || c.generation < 3 {
		t.Fatal("disconnect not invalidated")
	}
}

// TestCacheRefreshReusesOnlyVerifiedSpacing verifies cache refresh reuses only verified spacing.
//
// Version:
//   - 2026-09-08: Added.
func TestCacheRefreshReusesOnlyVerifiedSpacing(t *testing.T) {
	c, f := newTestCache(t)
	if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	first := f.calls
	if c.spacing != 60 {
		t.Fatal("spacing not retained")
	}
	if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	if f.calls-first != first-1 {
		t.Fatalf("expected one fewer read on disconnected refresh: first=%d second=%d", first, f.calls-first)
	}
	c2, f2 := newTestCache(t)
	f2.reorg = true
	if _, err := c2.QuotePair(context.Background(), big.NewInt(1000000), true); err == nil {
		t.Fatal("expected reorg rejection")
	}
	if c2.spacing != 0 {
		t.Fatal("unverified spacing retained")
	}
}

// TestBehindHeadSkipsAllContractReads verifies behind head skips all contract reads.
//
// Version:
//   - 2026-09-08: Added.
func TestBehindHeadSkipsAllContractReads(t *testing.T) {
	c, f := newTestCache(t)
	c.floor = 101
	if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err == nil {
		t.Fatal("expected lag rejection")
	}
	if f.calls != 0 {
		t.Fatalf("lagging head performed %d contract reads", f.calls)
	}
}

// TestCoveredLogDoesNotInvalidateButRemovedLogDoes verifies covered log does not invalidate but removed log does.
//
// Version:
//   - 2026-09-08: Added.
func TestCoveredLogDoesNotInvalidateButRemovedLogDoes(t *testing.T) {
	c, _ := newTestCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ws := &stateWS{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, ws) }()
	<-ws.ready
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		active := c.active
		c.mu.Unlock()
		if active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not active")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := c.QuotePair(ctx, big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	gen := c.generation
	h := c.covered
	c.mu.Unlock()
	ws.logs <- types.Log{Address: c.pool, BlockNumber: h.Number, BlockHash: h.Hash}
	ws.logs <- types.Log{Address: c.pool, BlockNumber: h.Number, BlockHash: h.Hash, Removed: true}
	for {
		c.mu.Lock()
		current := c.generation
		c.mu.Unlock()
		if current > gen {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("removed log not processed")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// One removal and one disconnect; the covered non-removed log does not count.
	if c.generation != gen+2 {
		t.Fatalf("generation=%d want=%d", c.generation, gen+2)
	}
}

type observedStateFake struct {
	*stateFake
	headers       int
	header        evm.BlockHeader
	err           error
	finalMismatch bool
}

// HeaderByNumber implements the injected test dependency.
//
// Version:
//   - 2026-09-08: Added.
func (f *observedStateFake) HeaderByNumber(_ context.Context, number uint64) (evm.BlockHeader, error) {
	f.headers++
	if number != 101 {
		f.t.Fatalf("unexpected block: %d", number)
	}
	if f.err != nil {
		return evm.BlockHeader{}, f.err
	}
	h := f.header
	if f.finalMismatch && f.headers > 1 {
		h.Hash = common.HexToHash("03")
	}
	return h, nil
}

// TestObservedBlockRecoversLagAndReusesSnapshot verifies observed block recovers lag and reuses snapshot.
//
// Version:
//   - 2026-09-08: Added.
func TestObservedBlockRecoversLagAndReusesSnapshot(t *testing.T) {
	c, base := newTestCache(t)
	c.active, c.floor, c.floorHash = true, 101, common.HexToHash("02")
	f := &observedStateFake{stateFake: base, header: evm.BlockHeader{Number: 101, Hash: c.floorHash}}
	c.rpc = f
	got, err := c.QuotePair(context.Background(), big.NewInt(1000000), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.BlockNumber != 101 || got.BlockHash != c.floorHash || f.headers != 2 {
		t.Fatalf("unexpected quote or header count: quote=%+v headers=%d", got, f.headers)
	}
	for _, n := range base.blocks {
		if n != 101 {
			t.Fatalf("state read at wrong block: %d", n)
		}
	}
	calls := base.calls
	if _, err = c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	if f.headers != 2 || base.calls != calls {
		t.Fatal("verified snapshot was not reused")
	}
}

// TestObservedBlockRecoveryRejectsUnsafeState verifies observed block recovery rejects unsafe state.
//
// Version:
//   - 2026-09-08: Added.
func TestObservedBlockRecoveryRejectsUnsafeState(t *testing.T) {
	for _, scenario := range []string{"missing_hash", "disconnected", "not_found", "deadline", "wrong_number", "wrong_hash", "reorg", "invalidation"} {
		t.Run(scenario, func(t *testing.T) {
			c, base := newTestCache(t)
			c.active, c.floor, c.floorHash = true, 101, common.HexToHash("02")
			f := &observedStateFake{stateFake: base, header: evm.BlockHeader{Number: 101, Hash: c.floorHash}}
			c.rpc = f
			switch scenario {
			case "missing_hash":
				c.floorHash = common.Hash{}
			case "disconnected":
				c.active = false
			case "not_found":
				f.err = ethereum.NotFound
			case "deadline":
				f.err = context.DeadlineExceeded
			case "wrong_number":
				f.header.Number = 100
			case "wrong_hash":
				f.header.Hash = common.HexToHash("03")
			case "reorg":
				f.finalMismatch = true
			case "invalidation":
				base.hook = func() { c.mu.Lock(); c.generation++; c.mu.Unlock() }
			}
			_, err := c.QuotePair(context.Background(), big.NewInt(1000000), true)
			if err == nil || c.snapshot != nil || c.spacing != 0 {
				t.Fatal("unsafe state accepted or cached")
			}
			if f.err != nil && !errors.Is(err, f.err) {
				t.Fatalf("underlying error lost: %v", err)
			}
			if scenario != "reorg" && scenario != "invalidation" && base.calls != 0 {
				t.Fatal("unsafe header allowed contract reads")
			}
			if f.headers > 2 {
				t.Fatal("unbounded header retries")
			}
		})
	}
}

// TestObservedHashTracksLogsAndClearsOnRemovalAndDisconnect verifies observed hash tracks logs and clears on removal and disconnect.
//
// Version:
//   - 2026-09-08: Added.
func TestObservedHashTracksLogsAndClearsOnRemovalAndDisconnect(t *testing.T) {
	c, _ := newTestCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ws := &stateWS{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, ws) }()
	<-ws.ready
	wait := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for {
			c.mu.Lock()
			ok := check()
			c.mu.Unlock()
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("subscription update timed out")
			}
			time.Sleep(time.Millisecond)
		}
	}
	wait(func() bool { return c.active })
	for _, log := range []types.Log{
		{BlockNumber: 101, BlockHash: common.HexToHash("01")},
		{BlockNumber: 101, BlockHash: common.HexToHash("01"), Removed: true},
		{BlockNumber: 101, BlockHash: common.HexToHash("02")},
		{BlockNumber: 100, BlockHash: common.HexToHash("03"), Removed: true},
		{BlockNumber: 102, BlockHash: common.HexToHash("04")},
	} {
		c.mu.Lock()
		gen := c.generation
		c.mu.Unlock()
		log.Address = c.pool
		ws.logs <- log
		wait(func() bool { return c.generation > gen })
		c.mu.Lock()
		h := c.floorHash
		c.mu.Unlock()
		want := log.BlockHash
		if log.Removed {
			want = common.Hash{}
		}
		if h != want {
			t.Fatalf("hash=%s want=%s", h, want)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.floorHash != (common.Hash{}) {
		t.Fatal("disconnected hash retained")
	}
}

func mustABIType(t *testing.T, name string) abi.Type {
	t.Helper()
	typ, err := abi.NewType(name, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

// TestDynamicFeeCapturedOnceAndRefreshedWithSnapshot verifies dynamic fee captured once and refreshed with snapshot.
//
// Version:
//   - 2026-09-08: Added.
func TestDynamicFeeCapturedOnceAndRefreshedWithSnapshot(t *testing.T) {
	c, f := newTestCache(t)
	c.active = true
	f.fee = big.NewInt(0)
	amount := big.NewInt(1000000)
	first, err := c.QuotePair(context.Background(), amount, true)
	if err != nil {
		t.Fatal(err)
	}
	if first.FeePPM != 0 {
		t.Fatal("zero fee lost")
	}
	f.fee = big.NewInt(100_000)
	reused, err := c.QuotePair(context.Background(), amount, true)
	if err != nil {
		t.Fatal(err)
	}
	if reused.FeePPM != 0 || !reused.ObservedAt.Equal(first.ObservedAt) || reused.BidAmountOut.Cmp(first.BidAmountOut) != 0 {
		t.Fatal("mixed fee into old snapshot")
	}
	c.snapshot.observed = time.Now().Add(-2 * time.Minute)
	refreshed, err := c.QuotePair(context.Background(), amount, true)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.FeePPM != 100_000 || refreshed.BidAmountOut.Cmp(first.BidAmountOut) >= 0 || refreshed.AskAmountIn.Cmp(first.AskAmountIn) <= 0 {
		t.Fatal("dynamic fee did not affect both sides")
	}
	f.feeError = errors.New("fee unavailable")
	c.generation++
	if _, err := c.QuotePair(context.Background(), amount, true); !errors.Is(err, f.feeError) {
		t.Fatalf("fee failure not preserved: %v", err)
	}
	if c.snapshot != nil {
		t.Fatal("reused failed fee state")
	}
}

// TestRejectInvalidDynamicFee verifies reject invalid dynamic fee.
//
// Version:
//   - 2026-09-08: Added.
func TestRejectInvalidDynamicFee(t *testing.T) {
	for _, fee := range []*big.Int{big.NewInt(1_000_000), power2(24)} {
		c, f := newTestCache(t)
		f.fee = fee
		if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err == nil {
			t.Fatal("invalid fee accepted")
		}
	}
}

// TestStakedLiquidityDoesNotReplaceTotalLiquidityNet verifies staked liquidity does not replace total liquidity net.
//
// Version:
//   - 2026-09-08: Added.
func TestStakedLiquidityDoesNotReplaceTotalLiquidityNet(t *testing.T) {
	for _, direction := range []bool{true, false} {
		c, f := newTestCache(t)
		f.crossed = true
		first, err := c.QuotePair(context.Background(), integer("10000000000000000"), direction)
		if err != nil {
			t.Fatal(err)
		}
		f.stakingNet = integer("50000000000000000")
		next, err := c.QuotePair(context.Background(), integer("10000000000000000"), direction)
		if err != nil {
			t.Fatal(err)
		}
		if first.BidAmountOut.Cmp(next.BidAmountOut) != 0 || first.AskAmountIn.Cmp(next.AskAmountIn) != 0 {
			t.Fatal("staking fee distribution changed trader amount")
		}
	}
}

// TestFactoryNotificationInvalidatesSnapshot verifies factory notification invalidates snapshot.
//
// Version:
//   - 2026-09-08: Added.
func TestFactoryNotificationInvalidatesSnapshot(t *testing.T) {
	c, _ := newTestCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	ws := &stateWS{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, ws) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	<-ws.ready
	ws.logs <- types.Log{Address: c.factory, BlockNumber: 101, BlockHash: common.HexToHash("03")}
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		floor := c.floor
		hash := c.floorHash
		c.mu.Unlock()
		if floor == 101 && hash == common.HexToHash("03") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("factory update ignored")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNewStateCacheComposition verifies new state cache composition.
//
// Version:
//   - 2026-09-08: Added.
func TestNewStateCacheComposition(t *testing.T) {
	f := &stateFake{t: t}
	pool, factory := common.HexToAddress("01"), common.HexToAddress("02")
	c, err := NewStateCache(f, pool, factory, time.Minute)
	if err != nil || c.rpc != f || c.pool != pool || c.factory != factory || cap(c.gate) != 1 {
		t.Fatalf("composition: %v", err)
	}
	for _, tc := range []struct {
		rpc           StateRPC
		pool, factory common.Address
		age           time.Duration
	}{
		{nil, pool, factory, time.Minute}, {f, common.Address{}, factory, time.Minute}, {f, pool, common.Address{}, time.Minute}, {f, pool, factory, 0},
	} {
		if _, err := NewStateCache(tc.rpc, tc.pool, tc.factory, tc.age); err == nil {
			t.Fatal("invalid constructor accepted")
		}
	}
}

// TestRejectV3TickLayout verifies reject v3 tick layout.
//
// Version:
//   - 2026-09-08: Added.
func TestRejectV3TickLayout(t *testing.T) {
	c, _ := newTestCache(t)
	c.rpc = &shortTickRPC{stateFake: &stateFake{t: t, crossed: true}}
	if _, err := c.QuotePair(context.Background(), integer("10000000000000000"), true); err == nil {
		t.Fatal("v3 tick ABI accepted")
	}
}

type shortTickRPC struct{ *stateFake }

// CallContract implements the injected test dependency.
//
// Version:
//   - 2026-09-08: Added.
func (r *shortTickRPC) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	data, err := r.stateFake.CallContract(ctx, msg, block)
	if err == nil && string(msg.Data[:4]) == string(methodSelector("ticks(int24)")) {
		return data[:256], nil
	}
	return data, err
}
