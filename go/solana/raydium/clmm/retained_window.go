package clmm

import (
	"context"
	"fmt"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"time"
)

// retainedWindowMissing checks the bounded array identities around the current position.
func (s *StateCache) retainedWindowMissing(ctx context.Context, requests []ExactInputRequest) bool {
	frozen := s.retained.Freeze()
	if len(frozen.Accounts) == 0 {
		return false
	}
	accounts := make(snapshotAccounts, len(frozen.Accounts))
	for address, value := range frozen.Accounts {
		accounts[address] = value.Account
	}
	local := &Client{accounts: accounts, programID: s.client.programID, pools: s.client.pools}
	pool, err := local.refreshPool(ctx, s.client.pools[s.pool])
	if err != nil {
		return false
	} // Invalid pool inputs cannot be repaired by fetching neighboring arrays.
	initial, _, err := solana.NormalizeQuoteArrayCounts(s.client.initialArrayCount, s.client.maxArrayCount)
	if err != nil {
		return false
	}
	_, candidates, err := local.quoteArrayCandidates(pool, windowDirections(pool))
	if err != nil {
		return false
	}
	for _, address := range solana.SelectQuoteArrays(candidates, nil, initial) {
		if accounts[address] == nil {
			return true
		}
	}
	return false
}

func (s *StateCache) runRetainedWindow(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	backoff := time.Second
	var next time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if time.Now().Before(next) {
			continue
		}
		if !s.retainedWindowMissing(ctx, nil) {
			continue
		}
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.captureRetainedWindow(refreshCtx, nil)
		cancel()
		if err != nil {
			next = time.Now().Add(backoff)
			backoff = min(30*time.Second, backoff*2)
		} else {
			next = time.Time{}
			backoff = time.Second
		}
	}
}

func (s *StateCache) captureRetainedWindow(ctx context.Context, requests []ExactInputRequest) error {
	s.refresh.Lock()
	defer s.refresh.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	minimum := s.minimumSlot
	s.mu.Unlock()
	local := &Client{initialArrayCount: s.client.initialArrayCount, maxArrayCount: s.client.maxArrayCount, accounts: s.client.accounts, programID: s.client.programID, pools: s.client.pools, configs: make(map[solana.Address]AMMConfig)}
	frozen := s.retained.Freeze()
	if account, ok := frozen.Accounts[s.pool]; ok {
		pool, err := decodePool(s.pool, account.Account.Data)
		if err != nil {
			return fmt.Errorf("failed to capture retained array window: %w", err)
		}
		local.quotePools.Store(s.pool, pool)
	}
	capture := &cacheSnapshotProvider{source: s.client.snapshots, local: local, pool: s.pool, minimum: minimum}
	local.snapshots = capture
	batch, err := local.captureArrayWindow(ctx, s.pool, windowDirections(s.client.pools[s.pool]))
	if err != nil {
		return fmt.Errorf("failed to capture retained array window: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return fmt.Errorf("failed to install retained array window: subscription=inactive")
	}
	accounts := make([]*solana.Account, 0, len(capture.accounts))
	for _, account := range capture.accounts {
		accounts = append(accounts, account)
	}
	// Full account payloads merge by slot; received updates at the same or a newer
	// slot win over bootstrap data. This intentionally does not assert atomicity.
	s.retained.SeedWindow(accounts, batch.Slot, s.now())
	if !sameAddresses(s.addresses, capture.addresses) {
		s.addresses = append([]solana.Address(nil), capture.addresses...)
		select {
		case s.changed <- struct{}{}:
		default:
		}
	}
	s.publishQuoteSnapshotLocked()
	return nil
}
