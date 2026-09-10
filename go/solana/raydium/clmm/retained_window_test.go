package clmm

import (
	"context"
	"encoding/binary"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"reflect"
	"slices"
	"testing"
	"time"
)

// TestRetainedWindowRefillsBeforeReferenceCrossing verifies discovery, batching and local-only consumers.
//
// Version:
//   - 2026-09-11: Added.
func TestRetainedWindowRefillsBeforeReferenceCrossing(t *testing.T) {
	c, source := cacheFixture(t)
	requests := []ExactInputRequest{{InputMint: testAddress(4), AmountIn: 1000}}
	ctx := context.Background()
	if _, err := c.QuoteExactInputs(ctx, requests); err != nil {
		t.Fatal(err)
	}
	baseline, direct := source.snapshotCalls, source.accountCalls
	if c.retainedWindowMissing(ctx, requests) {
		t.Fatal("initial arrays incomplete")
	}
	pool := *source.values[c.pool]
	pool.Data = append([]byte(nil), pool.Data...)
	binary.LittleEndian.PutUint64(pool.Data[904+7*8:912+7*8], uint64(1)<<63)
	source.values[c.pool] = &pool
	if err := c.retained.Apply(&solana.AccountUpdate{Slot: 100, Account: &pool}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.QuoteRetainedExactInputs(ctx, requests); err != nil {
		t.Fatal("reference quote unexpectedly needs adjacent array", err)
	}
	if !c.retainedWindowMissing(ctx, requests) {
		t.Fatal("adjacent coverage not prefetched")
	}
	if source.snapshotCalls != baseline || source.accountCalls != direct {
		t.Fatal("coverage check used RPC")
	}
	next, err := tickArrayAddress(mainnetProgramID, c.pool, -600)
	if err != nil {
		t.Fatal(err)
	}
	source.values[next] = &solana.Account{Address: next, Owner: mainnetProgramID, Data: testTickArrayData(c.pool, -600)}
	c.running = true
	before := c.retained.Freeze()
	saved := source.values[next]
	source.values[next] = nil
	if err := c.captureRetainedWindow(ctx, requests); err == nil {
		t.Fatal("missing capture account accepted")
	}
	if !reflect.DeepEqual(before, c.retained.Freeze()) {
		t.Fatal("failed capture modified live state")
	}
	source.values[next] = saved
	if err := c.captureRetainedWindow(ctx, requests); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(c.addresses, next) {
		t.Fatal("new array not subscribed")
	}
	if c.retained.Freeze().Accounts[c.pool].Slot != 100 {
		t.Fatal("capture overwrote newer live pool")
	}
	if c.retainedWindowMissing(ctx, requests) {
		t.Fatal("new array still missing")
	}
	baseline, direct = source.snapshotCalls, source.accountCalls
	if _, err := c.QuoteRetainedExactInputs(ctx, requests); err != nil {
		t.Fatal(err)
	}
	if source.snapshotCalls != baseline || source.accountCalls != direct {
		t.Fatal("local consumer invoked RPC")
	}
}
