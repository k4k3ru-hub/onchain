package clmm

import (
	"context"
	"encoding/binary"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"testing"
	"time"
)

// TestRetainedQuoteReadsUpdatedFeeWithoutRPC verifies retained snapshot behavior.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedQuoteReadsUpdatedFeeWithoutRPC(t *testing.T) {
	cache, source := cacheFixture(t)
	requests := []ExactInputRequest{{InputMint: testAddress(4), AmountIn: 1000000}}
	first, err := cache.QuoteExactInputs(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	calls := source.snapshotCalls
	config := *source.values[testAddress(2)]
	config.Data = append([]byte(nil), config.Data...)
	binary.LittleEndian.PutUint32(config.Data[47:51], 5000)
	if err := cache.retained.Apply(&solana.AccountUpdate{Slot: first.Slot + 1, Account: &config}, time.Now()); err != nil {
		t.Fatal(err)
	}
	cache.refresh.Lock()
	defer cache.refresh.Unlock()
	updated, err := cache.QuoteRetainedExactInputs(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Quotes[0].AmountOut >= first.Quotes[0].AmountOut {
		t.Fatal("retained calculation ignored streamed fee")
	}
	if source.snapshotCalls != calls {
		t.Fatal("retained calculation used RPC")
	}
}
