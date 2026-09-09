package dlmm

import (
	"context"
	"fmt"
	"time"
)

// QuoteRetainedExactInputs computes from a detached copy of the last received accounts.
// It never refreshes, waits for a stream position, or retries missing state. Component
// slots may differ; Slot is the oldest component position, not an atomic snapshot.
//
// Version:
//   - 2026-09-09: Keep local capture time separate from input provenance.
func (s *StateCache) QuoteRetainedExactInputs(ctx context.Context, requests []ExactInputRequest) (CachedQuote, error) {
	if s == nil || ctx == nil {
		return CachedQuote{}, fmt.Errorf("failed to quote retained state: dependency=null")
	}
	if err := ctx.Err(); err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote retained state: %w", err)
	}
	if len(requests) == 0 {
		return CachedQuote{}, fmt.Errorf("failed to quote retained state: requests=empty")
	}
	capturedAt := time.Now()
	frozen := s.retained.Freeze()
	if len(frozen.Accounts) == 0 {
		return CachedQuote{}, fmt.Errorf("failed to quote retained state: snapshot=null")
	}
	accounts := make(snapshotAccounts, len(frozen.Accounts))
	for address, value := range frozen.Accounts {
		accounts[address] = value.Account
	}
	local := &Client{accounts: accounts, programID: s.client.programID, pools: s.client.pools}

	result := CachedQuote{CapturedAt: capturedAt, QuoteBatch: QuoteBatch{Slot: frozen.Slot, Quotes: make([]Quote, len(requests))}, ObservedAt: frozen.ObservedAt}
	for i, request := range requests {
		quote, err := local.quoteExactInput(ctx, s.pool, request.InputMint, request.AmountIn)
		if err != nil {
			return CachedQuote{}, fmt.Errorf("failed to quote retained state: %w", err)
		}
		result.Quotes[i] = quote
	}
	if err := ctx.Err(); err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote retained state: %w", err)
	}
	return result, nil
}
