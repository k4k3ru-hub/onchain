package v3

import (
	"context"
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
