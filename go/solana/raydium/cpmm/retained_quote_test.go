package cpmm

import (
	"context"
	"errors"
	"fmt"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"testing"
	"time"
)

// TestRetainedQuoteUsesStreamAccountWithoutRPCOrRefreshLock verifies retained snapshot behavior.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedQuoteUsesStreamAccountWithoutRPCOrRefreshLock(t *testing.T) {
	c, rpc := cacheFixture(t)
	requests := []ExactInputRequest{{InputMint: testAddress(5), AmountIn: 100000}}
	first, err := c.QuoteExactInputs(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	calls := rpc.snapshotCalls
	// Simulate RPC refresh in flight. Retained math must never take that lock.
	c.refresh.Lock()
	defer c.refresh.Unlock()
	second, err := c.QuoteRetainedExactInputs(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	if second.Quotes[0] != first.Quotes[0] {
		t.Fatal("bootstrap quote mismatch")
	}
	if err := c.retained.Apply(&solana.AccountUpdate{Slot: first.Slot + 1, Account: tokenAccount(testAddress(4), 4_000_000)}, time.Now()); err != nil {
		t.Fatal(err)
	}
	third, err := c.QuoteRetainedExactInputs(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	if third.Quotes[0].AmountOut <= second.Quotes[0].AmountOut {
		t.Fatal("stream reserve did not affect quote")
	}
	if rpc.snapshotCalls != calls {
		t.Fatal("retained calculation used RPC")
	}
	if third.Slot != first.Slot || !third.ObservedAt.Equal(first.ObservedAt) {
		t.Fatal("relabelled older components")
	}
}

// TestRetainedQuoteMissingStateDoesNotBootstrap verifies retained snapshot behavior.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedQuoteMissingStateDoesNotBootstrap(t *testing.T) {
	c, rpc := cacheFixture(t)
	calls := rpc.snapshotCalls
	if _, err := c.QuoteRetainedExactInputs(context.Background(), []ExactInputRequest{{InputMint: testAddress(5), AmountIn: 1}}); err == nil {
		t.Fatal("accepted missing snapshot")
	}
	if rpc.snapshotCalls != calls {
		t.Fatal("bootstrapped from quote path")
	}
}

type retainedTestStream struct {
	update  *solana.AccountUpdate
	applied chan struct{}
}

func (s *retainedTestStream) Recv(context.Context) (solana.Slot, error) {
	return 0, fmt.Errorf("failed to receive test state: slot_only=invalid")
}
func (s *retainedTestStream) RecvState(ctx context.Context) (*solana.AccountUpdate, error) {
	if s.update != nil {
		result := s.update
		s.update = nil
		return result, nil
	}
	close(s.applied)
	<-ctx.Done()
	return nil, ctx.Err()
}
func (*retainedTestStream) Close() {}

// TestWatchAppliesFullAccountToRetainedQuote verifies retained snapshot behavior.
//
// Version:
//   - 2026-09-09: Added.
func TestWatchAppliesFullAccountToRetainedQuote(t *testing.T) {
	cache, source := cacheFixture(t)
	requests := []ExactInputRequest{{InputMint: testAddress(5), AmountIn: 100000}}
	first, err := cache.QuoteExactInputs(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	calls := source.snapshotCalls
	stream := &retainedTestStream{update: &solana.AccountUpdate{Slot: first.Slot + 1, Account: tokenAccount(testAddress(4), 4_000_000)}, applied: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- cache.watch(ctx, testAddress(4), func(solana.Address) (AccountChanges, error) { return stream, nil })
	}()
	select {
	case <-stream.applied:
	case <-time.After(time.Second):
		t.Fatal("full state not applied")
	}
	next, err := cache.QuoteRetainedExactInputs(ctx, requests)
	if err != nil {
		t.Fatal(err)
	}
	if next.Quotes[0].AmountOut <= first.Quotes[0].AmountOut || source.snapshotCalls != calls {
		t.Fatal("stream update not used locally")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected stream exit: %v", err)
	}
}
