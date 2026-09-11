package dlmm

import (
	"context"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

func (c *Client) captureArrayWindow(ctx context.Context, poolAddress onchainSolana.Address, requests []ExactInputRequest) (QuoteBatch, error) {
	if c == nil || c.snapshots == nil {
		return QuoteBatch{}, fmt.Errorf("failed to capture meteora dlmm array window: account_snapshot_provider=null")
	}
	if len(requests) == 0 {
		return QuoteBatch{}, fmt.Errorf("failed to capture meteora dlmm array window: requests=empty")
	}
	configured, exists := c.pools[poolAddress]
	if !exists {
		return QuoteBatch{}, fmt.Errorf("failed to capture meteora dlmm array window: pool=invalid pool_address=%q", poolAddress.String())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	initial, maximum, err := onchainSolana.NormalizeQuoteArrayCounts(c.initialArrayCount, c.maxArrayCount)
	if err != nil {
		return QuoteBatch{}, err
	}
	pool := configured
	if value, ok := c.quotePools.Load(poolAddress); ok {
		pool = value.(Pool)
	}
	var selected []onchainSolana.Address
	for attempt := 0; attempt < 5; attempt++ {
		if err := ctx.Err(); err != nil {
			return QuoteBatch{}, fmt.Errorf("failed to capture meteora dlmm array window: %w", err)
		}
		base, candidates, err := c.quoteArrayCandidates(pool, requests)
		if err != nil {
			return QuoteBatch{}, err
		}
		selected = onchainSolana.SelectQuoteArrays(candidates, selected, initial)
		if len(selected) > maximum {
			return QuoteBatch{}, fmt.Errorf("failed to capture meteora dlmm array window: %w: max_array_count=%d", onchainSolana.ErrQuoteArrayLimit, maximum)
		}
		addresses := append(base, selected...)
		snapshot, err := c.snapshots.AccountSnapshot(ctx, addresses)
		if err != nil {
			return QuoteBatch{}, fmt.Errorf("failed to capture meteora dlmm array window: %w", err)
		}
		accounts, err := newSnapshotAccounts(addresses, snapshot)
		if err != nil {
			return QuoteBatch{}, err
		}
		local := &Client{accounts: accounts, programID: c.programID, pools: c.pools, poolOrder: c.poolOrder}
		refreshed, err := local.refreshPool(ctx, configured)
		if err != nil {
			return QuoteBatch{}, err
		}
		pool = refreshed
		_, currentCandidates, err := c.quoteArrayCandidates(refreshed, requests)
		if err != nil {
			return QuoteBatch{}, err
		}
		current := onchainSolana.SelectQuoteArrays(currentCandidates, nil, initial)
		if sameAddresses(selected, current) {
			c.quotePools.Store(poolAddress, refreshed)
			return QuoteBatch{Slot: snapshot.Slot}, nil
		}
		selected = nil
	}
	return QuoteBatch{}, fmt.Errorf("failed to capture array window: %w: max_attempts=5", onchainSolana.ErrQuoteSnapshotLimit)
}

// Warm captures pool accounts and the bounded array window without calculating quotes.
//
// Version:
//   - 2026-09-12: Added.
func (s *StateCache) Warm(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("failed to warm retained state: cache=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.refresh.Lock()
	defer s.refresh.Unlock()
	s.mu.Lock()
	generation, minimum := s.generation, s.minimumSlot
	s.mu.Unlock()
	local := &Client{initialArrayCount: s.client.initialArrayCount, maxArrayCount: s.client.maxArrayCount, accounts: s.client.accounts, programID: s.client.programID, pools: s.client.pools}
	capture := &cacheSnapshotProvider{source: s.client.snapshots, minimum: minimum}
	local.snapshots = capture
	pool := s.client.pools[s.pool]
	requests := windowDirections(pool)
	batch, err := local.captureArrayWindow(ctx, s.pool, requests)
	if err != nil {
		return fmt.Errorf("failed to warm retained state: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("failed to warm retained state: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation {
		return fmt.Errorf("failed to warm retained state: %w", quotestate.ErrStateChanged)
	}
	observed := s.now()
	s.snapshot, s.observedAt = capture.accounts, observed
	accounts := make([]*onchainSolana.Account, 0, len(capture.accounts))
	for _, account := range capture.accounts {
		accounts = append(accounts, account)
	}
	s.retained.SeedWindow(accounts, batch.Slot, observed)
	s.minimumSlot = max(s.minimumSlot, batch.Slot)
	s.addresses = append([]onchainSolana.Address(nil), capture.addresses...)
	s.publishQuoteSnapshotLocked()
	return nil
}

// windowDirections selects both sides for address discovery; amounts are never quoted.
func windowDirections(pool Pool) []ExactInputRequest {
	return []ExactInputRequest{{InputMint: pool.TokenXMint, AmountIn: 1}, {InputMint: pool.TokenYMint, AmountIn: 1}}
}
