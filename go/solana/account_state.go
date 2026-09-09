package solana

import (
	"fmt"
	"sync"
	"time"
)

type RetainedAccount struct {
	Account    *Account
	Slot       Slot
	ObservedAt time.Time
}

type FrozenAccountState struct {
	Accounts   map[Address]RetainedAccount
	Slot       Slot
	ObservedAt time.Time
}

// AccountState retains complete account replacements independently by address.
// Its zero value is ready for use. It does not assert cross-account atomicity.
type AccountState struct {
	mu       sync.Mutex
	accounts map[Address]RetainedAccount
}

// Seed installs initial account data without overwriting newer stream observations.
//
// Version:
//   - 2026-09-09: Added.
func (s *AccountState) Seed(accounts []*Account, slot Slot, observed time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accounts == nil {
		s.accounts = make(map[Address]RetainedAccount)
	}
	for _, account := range accounts {
		if account == nil {
			continue
		}
		previous, exists := s.accounts[account.Address]
		if exists && previous.Slot >= slot {
			continue
		}
		s.accounts[account.Address] = copyRetainedAccount(RetainedAccount{account, slot, observed})
	}
}

// Apply retains a full account notification, preserving its individual slot and time.
// Older notifications cannot rewind an account; equal slots follow reception order.
//
// Version:
//   - 2026-09-09: Added.
func (s *AccountState) Apply(update *AccountUpdate, observed time.Time) error {
	if s == nil || update == nil || update.Account == nil {
		return fmt.Errorf("failed to apply account state: dependency=null")
	}
	if update.Account.Address.IsZero() {
		return fmt.Errorf("failed to apply account state: address=empty")
	}
	if err := update.Slot.Validate(); err != nil {
		return fmt.Errorf("failed to apply account state: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accounts == nil {
		s.accounts = make(map[Address]RetainedAccount)
	}
	if previous, exists := s.accounts[update.Account.Address]; exists && previous.Slot > update.Slot {
		return nil
	}
	s.accounts[update.Account.Address] = copyRetainedAccount(RetainedAccount{update.Account, update.Slot, observed})
	return nil
}

// Freeze copies all retained inputs without waiting for an RPC or matching slots.
// Slot and ObservedAt are conservative minima; per-account provenance is retained.
//
// Version:
//   - 2026-09-09: Added.
func (s *AccountState) Freeze() FrozenAccountState {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := FrozenAccountState{Accounts: make(map[Address]RetainedAccount, len(s.accounts))}
	for address, account := range s.accounts {
		result.Accounts[address] = copyRetainedAccount(account)
		if result.Slot == 0 || account.Slot < result.Slot {
			result.Slot = account.Slot
		}
		if result.ObservedAt.IsZero() || account.ObservedAt.Before(result.ObservedAt) {
			result.ObservedAt = account.ObservedAt
		}
	}
	return result
}

func copyRetainedAccount(value RetainedAccount) RetainedAccount {
	copied := *value.Account
	copied.Data = append([]byte(nil), copied.Data...)
	value.Account = &copied
	return value
}

// Reset discards all retained accounts after a disconnected state session.
// Previously frozen snapshots remain independent.
//
// Version:
//   - 2026-09-09: Added.
func (s *AccountState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts = nil
}
