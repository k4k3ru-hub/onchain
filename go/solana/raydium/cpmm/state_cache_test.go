package cpmm

import (
	"context"
	"encoding/binary"
	"errors"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
	"testing"
	"time"
)

func cacheFixture(t *testing.T) (*StateCache, *stubAccounts) {

	poolAddress := testAddress(1)
	configAddress := testAddress(2)
	vault0 := testAddress(3)
	vault1 := testAddress(4)
	mint0 := testAddress(5)
	mint1 := testAddress(6)

	poolData := make([]byte, poolDataLength)
	copy(poolData[:8], poolDiscriminator[:])
	putAddress(poolData, 8, configAddress)
	putAddress(poolData, 72, vault0)
	putAddress(poolData, 104, vault1)
	putAddress(poolData, 168, mint0)
	putAddress(poolData, 200, mint1)
	putAddress(poolData, 232, splTokenProgramID)
	putAddress(poolData, 264, splTokenProgramID)
	poolData[331] = 6
	poolData[332] = 6

	configData := make([]byte, ammConfigDataLength)
	copy(configData[:8], configDiscriminator[:])
	binary.LittleEndian.PutUint64(configData[12:20], 2_500)

	accounts := &stubAccounts{values: map[onchainSolana.Address]*onchainSolana.Account{
		poolAddress:   {Address: poolAddress, Owner: mainnetProgramID, Data: poolData},
		configAddress: {Address: configAddress, Owner: mainnetProgramID, Data: configData},
		vault0:        tokenAccount(vault0, 1_000_000),
		vault1:        tokenAccount(vault1, 2_000_000),
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
func TestStateCacheReuseInvalidationExpiryAndFailure(t *testing.T) {
	c, a := cacheFixture(t)
	c.connected = 4
	now := time.Now()
	c.now = func() time.Time { return now }
	req := []ExactInputRequest{{InputMint: testAddress(5), AmountIn: 100000}}
	first, err := c.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	baseline := a.snapshotCalls
	a.values[testAddress(4)].Data[tokenAmountOffset]++ // cached bytes must be detached
	second, err := c.QuoteExactInputs(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if a.snapshotCalls != baseline || first.Quotes[0] != second.Quotes[0] || !first.ObservedAt.Equal(second.ObservedAt) {
		t.Fatal("cache not reused or snapshot aliased")
	}
	c.mu.Lock()
	c.invalidate(42)
	c.mu.Unlock()
	if _, err = c.QuoteExactInputs(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if a.snapshotCalls != baseline+1 {
		t.Fatal("notification did not refresh")
	}
	now = now.Add(time.Minute)
	if _, err = c.QuoteExactInputs(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if a.snapshotCalls != baseline+2 {
		t.Fatal("expiry did not refresh")
	}
	c.mu.Lock()
	c.invalidate(43)
	c.mu.Unlock()
	if _, err = c.QuoteExactInputs(context.Background(), req); err == nil {
		t.Fatal("accepted RPC behind notification")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.QuoteExactInputs(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}

type cacheChanges struct {
	closed        chan struct{}
	notifications chan onchainSolana.Slot
}

// Recv waits for a test notification.
//
// Version:
//   - 2026-09-07: Added.
func (s *cacheChanges) Recv(ctx context.Context) (onchainSolana.Slot, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case n, ok := <-s.notifications:
		if !ok {
			return 0, errors.New("disconnected")
		}
		return n, nil
	}
}

// Close releases the test stream.
//
// Version:
//   - 2026-09-07: Added.
func (s *cacheChanges) Close() { close(s.closed) }
func TestStateCacheSubscriptionLifecycle(t *testing.T) {
	c, a := cacheFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subs := make(chan *cacheChanges, 4)
	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx, func(onchainSolana.Address) (AccountChanges, error) {
			s := &cacheChanges{make(chan struct{}), make(chan onchainSolana.Slot, 1)}
			subs <- s
			return s, nil
		})
	}()
	streams := make([]*cacheChanges, 4)
	for i := range streams {
		select {
		case streams[i] = <-subs:
		case <-time.After(time.Second):
			t.Fatal("subscription missing")
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		ready := c.connected == 4
		c.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not connected")
		}
		time.Sleep(time.Millisecond)
	}
	req := []ExactInputRequest{{InputMint: testAddress(5), AmountIn: 100000}}
	if _, err := c.QuoteExactInputs(ctx, req); err != nil {
		t.Fatal(err)
	}
	close(streams[0].notifications)
	select {
	case <-streams[0].closed:
	case <-time.After(time.Second):
		t.Fatal("stream not closed")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("disconnect error lost")
		}
	case <-time.After(time.Second):
		t.Fatal("workers leaked")
	}
	baseline := a.snapshotCalls
	for range 2 {
		if _, err := c.QuoteExactInputs(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if a.snapshotCalls != baseline+2 {
		t.Fatal("disconnected cache reused")
	}
}

type changingSnapshot struct {
	accounts *stubAccounts
	change   func()
	slot     onchainSolana.Slot
}

// AccountSnapshot returns a snapshot while simulating a concurrent notification.
//
// Version:
//   - 2026-09-07: Added.
func (p *changingSnapshot) AccountSnapshot(ctx context.Context, addresses []onchainSolana.Address) (*onchainSolana.AccountSnapshot, error) {
	value, err := p.accounts.AccountSnapshot(ctx, addresses)
	if err != nil {
		return nil, err
	}
	value.Slot = p.slot
	if p.change != nil {
		p.change()
	}
	return value, nil
}
func TestCacheRejectsConcurrentChangesAndSlotRegression(t *testing.T) {
	c, a := cacheFixture(t)
	c.connected = 4
	provider := &changingSnapshot{accounts: a, slot: 43}
	c.client.snapshots = provider
	req := []ExactInputRequest{{InputMint: testAddress(5), AmountIn: 100000}}
	provider.change = func() { c.mu.Lock(); defer c.mu.Unlock(); c.invalidate(43) }
	if _, err := c.QuoteExactInputs(context.Background(), req); err == nil {
		t.Fatal("concurrently invalidated snapshot accepted")
	}
	provider.change = nil
	if _, err := c.QuoteExactInputs(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.invalidate(0)
	c.mu.Unlock()
	provider.slot = 42
	if _, err := c.QuoteExactInputs(context.Background(), req); err == nil {
		t.Fatal("older snapshot accepted after reconnect")
	}
	provider.slot = 44
	if _, err := c.QuoteExactInputs(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}
