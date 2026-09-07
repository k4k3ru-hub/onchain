package v3

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
	t       *testing.T
	calls   int
	reorg   bool
	crossed bool
	hook    func()
	blocks  []uint64
}

func (f *stateFake) LatestHeader(context.Context) (evm.BlockHeader, error) {
	return evm.BlockHeader{Number: 100, Hash: common.HexToHash("01")}, nil
}
func (f *stateFake) HeaderByNumber(context.Context, uint64) (evm.BlockHeader, error) {
	h := common.HexToHash("01")
	if f.reorg {
		h = common.HexToHash("02")
	}
	return evm.BlockHeader{Number: 100, Hash: h}, nil
}
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
		args := abi.Arguments{{Type: mustABIType(f.t, "uint160")}, {Type: mustABIType(f.t, "int24")}, {Type: mustABIType(f.t, "uint16")}, {Type: mustABIType(f.t, "uint16")}, {Type: mustABIType(f.t, "uint16")}, {Type: mustABIType(f.t, "uint8")}, {Type: mustABIType(f.t, "bool")}}
		return args.Pack(power2(96), big.NewInt(0), uint16(0), uint16(0), uint16(0), uint8(0), true)
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
		data := make([]byte, 256)
		big.NewInt(1).FillBytes(data[:32])
		integer("100000000000000000").FillBytes(data[32:64])
		data[255] = 1
		return data, nil
	}
	return nil, fmt.Errorf("unexpected selector")
}
func newTestCache(t *testing.T) (*StateCache, *stateFake) {
	t.Helper()
	f := &stateFake{t: t}
	config := testFactoryConfig(t)
	client, err := NewHTTPClient(HTTPClientParams{RPC: &testHTTPRPC{}, Factory: config})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := config.PoolKeys[0].Address(config.Address, config.InitCodeHash)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewStateCache(client, f, pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}
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

func (w *stateWS) SubscribeFilterLogs(_ context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	if len(q.Topics) != 0 {
		return nil, fmt.Errorf("state events filtered")
	}
	w.logs = ch
	close(w.ready)
	return event.NewSubscription(func(quit <-chan struct{}) error { <-quit; return nil }), nil
}
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
