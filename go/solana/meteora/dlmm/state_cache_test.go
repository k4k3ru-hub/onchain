package dlmm

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	solana "github.com/k4k3ru-hub/onchain/go/solana"
)

func newCacheFixture(t *testing.T) (*StateCache, *observingAccounts, []ExactInputRequest) {
	t.Helper()
	source, pool, mint := adaptiveFixture(t)
	client, err := NewClient(context.Background(), source, Config{Pools: []solana.Address{pool}})
	if err != nil {
		t.Fatal(err)
	}
	cache, err := NewStateCache(client, pool, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return cache, source, []ExactInputRequest{{InputMint: mint, AmountIn: 1000}}
}

func enableCache(c *StateCache) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = len(c.addresses)
	for _, a := range c.addresses {
		c.active[a] = true
	}
}

func TestStateCacheReuseDetachedClockAndExpiry(t *testing.T) {
	c, source, req := newCacheFixture(t)
	now := time.Now()
	c.now = func() time.Time { return now }
	first, err := c.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	enableCache(c)
	calls := len(source.sizes)
	// Corrupt the provider's buffer. The cached Clock must remain detached and
	// must not be silently replaced with host wall time.
	binary.LittleEndian.PutUint64(source.values[clockSysvarAddress].Data[32:], ^uint64(0))
	second, err := c.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.sizes) != calls || first.Quotes[0] != second.Quotes[0] || first.Slot != second.Slot || !first.ObservedAt.Equal(second.ObservedAt) {
		t.Fatal("cache changed without refresh")
	}
	clockWatched := false
	for _, address := range c.addresses {
		if address == clockSysvarAddress {
			clockWatched = true
		}
	}
	if !clockWatched {
		t.Fatal("chain clock is not streamed")
	}
	now = now.Add(30 * time.Second)
	if _, err = c.QuoteExactInputs(context.Background(), req); err == nil {
		t.Fatal("expired cache hid invalid chain clock")
	}
	binary.LittleEndian.PutUint64(source.values[clockSysvarAddress].Data[32:], 1001)
	refreshed, err := c.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed.ObservedAt.Equal(now) || refreshed.Slot <= first.Slot {
		t.Fatal("expiry did not refresh snapshot")
	}
}

func TestStateCacheInvalidationAndSlotRegression(t *testing.T) {
	c, source, req := newCacheFixture(t)
	if _, err := c.QuoteExactInputs(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	enableCache(c)
	c.mu.Lock()
	c.invalidate(100)
	c.mu.Unlock()
	if _, err := c.QuoteExactInputs(context.Background(), req); err == nil {
		t.Fatal("accepted snapshot behind notification")
	}
	if len(source.sizes) != 2 {
		t.Fatal("did not refresh invalidated snapshot")
	}
}

func TestStateCacheConcurrentNotificationRejectsRefresh(t *testing.T) {
	c, source, req := newCacheFixture(t)
	source.mutate = func(int) { c.mu.Lock(); c.invalidate(0); c.mu.Unlock() }
	if _, err := c.QuoteExactInputs(context.Background(), req); err == nil {
		t.Fatal("accepted refresh raced by notification")
	}
	if c.snapshot != nil {
		t.Fatal("raced snapshot retained")
	}
}

func TestStateCacheWithoutSubscriptionsRefreshes(t *testing.T) {
	c, source, req := newCacheFixture(t)
	for i := 0; i < 2; i++ {
		if _, err := c.QuoteExactInputs(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if len(source.sizes) != 2 {
		t.Fatal("reused cache without subscriptions")
	}
}

type cacheChanges struct {
	events chan solana.Slot
	closed chan struct{}
}

func (c *cacheChanges) Recv(ctx context.Context) (solana.Slot, error) {
	select {
	case slot := <-c.events:
		return slot, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
func (c *cacheChanges) Close() { close(c.closed) }

func TestStateCacheRunReconfiguresAndClosesSubscriptions(t *testing.T) {
	c, _, req := newCacheFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := make(chan *cacheChanges, 8)
	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx, func(solana.Address) (AccountChanges, error) {
			s := &cacheChanges{events: make(chan solana.Slot), closed: make(chan struct{})}
			opened <- s
			return s, nil
		})
	}()
	receive := func() *cacheChanges {
		t.Helper()
		select {
		case s := <-opened:
			return s
		case <-time.After(2 * time.Second):
			t.Fatal("subscription not opened")
			return nil
		}
	}
	first := receive()
	// Wait for initial registration, so the refresh doesn't race registration.
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.mu.Lock()
		ready := c.connected == 1
		c.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not registered")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := c.QuoteExactInputs(ctx, req); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("old session not closed")
	}
	second, third := receive(), receive() // pool and selected bin array
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("workers not joined")
	}
	for _, s := range []*cacheChanges{second, third} {
		select {
		case <-s.closed:
		default:
			t.Fatal("leaked subscription")
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshot != nil || c.connected != 0 || c.running {
		t.Fatal("cache remained live after close")
	}
}

func TestStateCacheExpandsForLargerQuote(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)
	binary.LittleEndian.PutUint64(source.values[pool].Data[584+8*8:], 3)
	first, err := binArrayAddress(mainnetProgramID, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint64(source.values[first].Data[56+5*binDataLength+8:], 100)
	next, err := binArrayAddress(mainnetProgramID, pool, 0)
	if err != nil {
		t.Fatal(err)
	}
	data := testBinArrayData(pool, 0)
	binary.LittleEndian.PutUint64(data[56+69*binDataLength+8:], 10_000_000)
	binary.LittleEndian.PutUint64(data[56+69*binDataLength+24:], 1)
	source.values[next] = &solana.Account{Address: next, Owner: mainnetProgramID, Data: data}
	client, err := NewClient(context.Background(), source, Config{Pools: []solana.Address{pool}, InitialArrayCount: 1, MaxArrayCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewStateCache(client, pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.QuoteExactInputs(context.Background(), []ExactInputRequest{{InputMint: mint, AmountIn: 10}}); err != nil {
		t.Fatal(err)
	}
	enableCache(c)
	expanded, err := c.QuoteExactInputs(context.Background(), []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	if expanded.Quotes[0].BinArraysUsed != 2 || len(c.addresses) != 4 {
		t.Fatal("larger quote did not expand cached arrays")
	}
	full, err := client.QuoteExactInputsWithSlot(context.Background(), pool, []ExactInputRequest{{InputMint: mint, AmountIn: 1_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	if full.Quotes[0] != expanded.Quotes[0] {
		t.Fatal("cache quote differs from coherent full snapshot")
	}
}
