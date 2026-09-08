package clmm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"sort"
)

// Changes returns coalesced account and subscription lifecycle notifications.
// The channel has one consumer and is never closed.
//
// Version:
//   - 2026-09-09: Added.
func (s *StateCache) Changes() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.updates == nil {
		s.updates = make(chan struct{}, 1)
	}
	return s.updates
}
func (s *StateCache) signalChange() {
	select {
	case s.updates <- struct{}{}:
	default:
	}
}

// CheckState verifies every tracked account in one RPC context without swap calculation.
//
// Version:
//   - 2026-09-09: Added.
func (s *StateCache) CheckState(ctx context.Context) (result quotestate.Check, err error) {
	if s == nil || ctx == nil {
		return result, fmt.Errorf("failed to check clmm state: dependency=null")
	}
	s.refresh.Lock()
	defer s.refresh.Unlock()
	if err = ctx.Err(); err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	s.mu.Lock()
	gen, minimum := s.generation, s.minimumSlot
	addresses := append([]solana.Address(nil), s.addresses...)
	if len(s.snapshot) > 0 {
		addresses = addresses[:0]
		for address := range s.snapshot {
			addresses = append(addresses, address)
		}
	}
	s.mu.Unlock()
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].String() < addresses[j].String() })
	started := s.now()
	value, err := s.client.snapshots.AccountSnapshot(ctx, addresses)
	if err != nil {
		return result, fmt.Errorf("failed to check clmm state: %w", err)
	}
	if value == nil || value.Slot == 0 || value.Slot < minimum || len(value.Accounts) != len(addresses) {
		return result, fmt.Errorf("failed to check clmm state: snapshot=invalid")
	}
	accounts := make(map[string]*solana.Account, len(addresses))
	for i, a := range value.Accounts {
		if a == nil {
			return result, fmt.Errorf("failed to check clmm state: account=null")
		}
		detached := *a
		detached.Data = append([]byte(nil), a.Data...)
		accounts[addresses[i].String()] = &detached
	}
	encoded, err := json.Marshal(accounts)
	if err != nil {
		return result, fmt.Errorf("failed to encode clmm state: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	key := fmt.Sprintf("%x", sha256.Sum256(encoded))
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.generation {
		return result, fmt.Errorf("failed to check clmm state: snapshot=invalidated")
	}
	if s.checkedKey == key {
		s.snapshot = make(snapshotAccounts, len(addresses))
		for _, address := range addresses {
			s.snapshot[address] = accounts[address.String()]
		}
		s.observedAt = started
	} else {
		s.snapshot = nil
	}
	s.checkedKey = key
	s.minimumSlot = value.Slot
	return quotestate.Check{Revision: gen, Key: key, Position: value.Slot.Uint64(), CheckedAt: started}, nil
}

// CheckCurrent reports whether notifications received since verification invalidate the check.
//
// Version:
//   - 2026-09-09: Added.
func (s *StateCache) CheckCurrent(check quotestate.Check) bool {
	if s == nil || check.Key == "" || check.Position == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation == check.Revision
}
