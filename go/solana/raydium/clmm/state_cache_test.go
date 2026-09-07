package clmm

import (
	"context"
	"encoding/binary"
	"fmt"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
	"math/big"
	"testing"
	"time"
)

func cacheFixture(t *testing.T) (*StateCache, *stubAccounts) {
	poolAddress := testAddress(1)
	configAddress := testAddress(2)
	token0, token1 := testAddress(4), testAddress(5)
	poolData := make([]byte, poolDataLength)
	copy(poolData[:8], poolDiscriminator[:])
	putAddress(poolData, 9, configAddress)
	putAddress(poolData, 73, token0)
	putAddress(poolData, 105, token1)
	putAddress(poolData, 137, testAddress(6))
	putAddress(poolData, 169, testAddress(7))
	binary.LittleEndian.PutUint16(poolData[235:237], 10)
	putUint128LE(poolData[237:253], big.NewInt(1_000_000_000_000))
	price, err := sqrtPriceAtTick(5)
	if err != nil {
		t.Fatalf("sqrtPriceAtTick() error = %v", err)
	}
	putUint128LE(poolData[253:269], price)
	binary.LittleEndian.PutUint32(poolData[269:273], 5)
	binary.LittleEndian.PutUint64(poolData[904+8*8:912+8*8], 1)
	tickAddress, err := tickArrayAddress(mainnetProgramID, poolAddress, 0)
	if err != nil {
		t.Fatalf("tickArrayAddress() error = %v", err)
	}
	tickData := testTickArrayData(poolAddress, 0)
	putUint128LE(tickData[44+20:44+36], big.NewInt(1))
	tickData[10124] = 1
	configData := testAMMConfigData(10)
	binary.LittleEndian.PutUint32(configData[47:51], 2500)
	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:   {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		configAddress: {Address: configAddress, Owner: mainnetProgramID, Data: configData},
		tickAddress:   {Address: tickAddress, Owner: mainnetProgramID, Data: tickData},
	}}
	client, err := NewClient(context.Background(), accounts, Config{Pools: []onchainSolana.Address{poolAddress}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	cache, err := NewStateCache(client, poolAddress, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return cache, accounts
}
func TestCLMMCacheReuseAndConfigInvalidation(t *testing.T) {
	c, a := cacheFixture(t)
	now := time.Now()
	c.now = func() time.Time { return now }
	req := []ExactInputRequest{{InputMint: testAddress(4), AmountIn: 1000000}}
	first, err := c.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	c.connected = len(c.addresses)
	for _, address := range c.addresses {
		c.active[address] = true
	}
	baseline := a.snapshotCalls
	second, err := c.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if a.snapshotCalls != baseline || first.Quotes[0] != second.Quotes[0] || !first.ObservedAt.Equal(second.ObservedAt) {
		t.Fatal("snapshot not reused")
	}
	binary.LittleEndian.PutUint32(a.values[testAddress(2)].Data[47:51], 5000)
	unchanged, err := c.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Quotes[0] != first.Quotes[0] {
		t.Fatal("cached config aliased")
	}
	c.invalidate(1)
	updated, err := c.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Quotes[0].TradeFee <= first.Quotes[0].TradeFee {
		t.Fatal("config change ignored")
	}
	now = now.Add(time.Minute)
	if _, err := c.QuoteExactInputs(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if a.snapshotCalls != baseline+2 {
		t.Fatal("expiry refresh missing")
	}
	c.invalidate(2)
	if _, err := c.QuoteExactInputs(context.Background(), req); err == nil {
		t.Fatal("regressed slot accepted")
	}
}

func TestCacheExpandsArraysForLargerAmount(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)

	poolData := source.values[pool].Data
	// Current array 0 and the next initialized array -600.
	binary.LittleEndian.PutUint64(poolData[904+7*8:912+7*8], uint64(1)<<63)
	next, err := tickArrayAddress(mainnetProgramID, pool, -600)
	if err != nil {
		t.Fatal(err)
	}
	data := testTickArrayData(pool, -600)
	negativeTick := int32(-600)
	binary.LittleEndian.PutUint32(data[44:48], uint32(negativeTick))
	putUint128LE(data[44+20:44+36], big.NewInt(1))
	data[10124] = 1
	source.values[next] = &onchainSolana.Account{Address: next, Owner: mainnetProgramID, Data: data}

	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}, InitialArrayCount: 1, MaxArrayCount: 2})
	if err != nil {
		t.Fatal(err)
	}

	cache, err := NewStateCache(client, pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req := []ExactInputRequest{{InputMint: mint, AmountIn: 1000000}}
	if _, err := cache.QuoteExactInputs(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	cache.connected = len(cache.addresses)
	for _, a := range cache.addresses {
		cache.active[a] = true
	}
	calls := len(source.sizes)
	req[0].AmountIn = 1000000000
	quote, err := cache.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.sizes) <= calls || quote.Quotes[0].TickArraysUsed != 2 || len(cache.addresses) != 4 {
		t.Fatalf("missing adaptive expansion: sizes=%v quote=%+v addresses=%d", source.sizes, quote, len(cache.addresses))
	}
	if cache.active[next] {
		t.Fatal("new tick array incorrectly marked subscribed")
	}
}

type cacheStream struct {
	slots  chan onchainSolana.Slot
	closed chan struct{}
}

// Recv waits for a fake notification or cancellation.
//
// Version:
//   - 2026-09-07: Added.
func (s *cacheStream) Recv(ctx context.Context) (onchainSolana.Slot, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case slot, ok := <-s.slots:
		if !ok {
			return 0, fmt.Errorf("disconnected")
		}
		return slot, nil
	}
}

// Close signals fake stream shutdown.
//
// Version:
//   - 2026-09-07: Added.
func (s *cacheStream) Close() { close(s.closed) }
func TestCacheSubscriptionsFollowSnapshotArrays(t *testing.T) {
	cache, a := cacheFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	streams := make(chan *cacheStream, 20)
	go func() {
		done <- cache.Run(ctx, func(onchainSolana.Address) (AccountChanges, error) {
			s := &cacheStream{make(chan onchainSolana.Slot), make(chan struct{})}
			streams <- s
			return s, nil
		})
	}()
	wait := func(n int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			cache.mu.Lock()
			ready := cache.connected == n && len(cache.active) == n
			cache.mu.Unlock()
			if ready {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("subscriptions not ready")
			}
			time.Sleep(time.Millisecond)
		}
	}
	wait(2)
	req := []ExactInputRequest{{InputMint: testAddress(4), AmountIn: 1000000}}
	if _, err := cache.QuoteExactInputs(ctx, req); err != nil {
		t.Fatal(err)
	}
	wait(3)
	if _, err := cache.QuoteExactInputs(ctx, req); err != nil {
		t.Fatal(err)
	}
	baseline := a.snapshotCalls
	if _, err := cache.QuoteExactInputs(ctx, req); err != nil {
		t.Fatal(err)
	}
	if a.snapshotCalls != baseline {
		t.Fatal("subscribed cache not reused")
	}
	var live *cacheStream
	for len(streams) > 0 {
		s := <-streams
		select {
		case <-s.closed:
		default:
			live = s
		}
	}
	if live == nil {
		t.Fatal("live stream missing")
	}
	close(live.slots)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stream failure lost")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop")
	}
	if _, err := cache.QuoteExactInputs(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if a.snapshotCalls != baseline+1 {
		t.Fatal("disconnected cache reused")
	}
}

func TestCacheRejectsNotificationDuringRefresh(t *testing.T) {
	source, pool, mint := adaptiveFixture(t)
	client, err := NewClient(context.Background(), source, Config{Pools: []onchainSolana.Address{pool}})
	if err != nil {
		t.Fatal(err)
	}
	cache, err := NewStateCache(client, pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	source.mutate = func(int) { cache.mu.Lock(); defer cache.mu.Unlock(); cache.invalidate(1) }
	req := []ExactInputRequest{{InputMint: mint, AmountIn: 1000000}}
	if _, err := cache.QuoteExactInputs(context.Background(), req); err == nil {
		t.Fatal("concurrently invalidated snapshot accepted")
	}
	source.mutate = nil
	if _, err := cache.QuoteExactInputs(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}
