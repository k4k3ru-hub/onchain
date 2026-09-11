package cpmm

import (
	"context"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
)

// Warm captures the pool, configuration and vault accounts without calculating quotes.
//
// Version:
//   - 2026-09-12: Added.
func (s *StateCache) Warm(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("failed to warm cpmm state: cache=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("failed to warm cpmm state: %w", err)
	}
	s.refresh.Lock()
	defer s.refresh.Unlock()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("failed to warm cpmm state: %w", err)
	}
	s.mu.Lock()
	snapshot, observed := s.snapshot, s.observedAt
	generation, minimum := s.generation, s.minimumSlot
	valid := snapshot != nil && s.connected == len(s.addresses) && s.now().Sub(observed) < s.maxAge
	s.mu.Unlock()
	if !valid {
		fetched, err := s.client.snapshots.AccountSnapshot(ctx, s.addresses)
		if err != nil {
			return fmt.Errorf("failed to refresh cpmm state: %w", err)
		}
		if fetched == nil || len(fetched.Accounts) != len(s.addresses) {
			return fmt.Errorf("failed to refresh cpmm state: snapshot=invalid")
		}
		if err := fetched.Slot.Validate(); err != nil {
			return fmt.Errorf("failed to refresh cpmm state: %w", err)
		}
		if fetched.Slot < minimum {
			return fmt.Errorf("failed to refresh cpmm state: snapshot behind observed state: snapshot_slot=%d minimum_slot=%d", fetched.Slot, minimum)
		}
		snapshot = &solana.AccountSnapshot{Slot: fetched.Slot, Accounts: make([]*solana.Account, len(fetched.Accounts))}
		for i, a := range fetched.Accounts {
			if a == nil {
				return fmt.Errorf("failed to refresh cpmm state: account=null")
			}
			copied := *a
			copied.Data = append([]byte(nil), a.Data...)
			snapshot.Accounts[i] = &copied
		}
		observed = s.now()
	}
	if _, err := newSnapshotAccounts(s.addresses, snapshot); err != nil {
		return fmt.Errorf("failed to warm cpmm state: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation {
		return fmt.Errorf("failed to warm cpmm state: %w: state changed during calculation", quotestate.ErrStateChanged)
	}
	s.snapshot = snapshot
	s.retained.Seed(snapshot.Accounts, snapshot.Slot, observed)
	s.publishQuoteSnapshotLocked()
	if snapshot.Slot > s.minimumSlot {
		s.minimumSlot = snapshot.Slot
	}
	s.observedAt = observed
	return nil
}
