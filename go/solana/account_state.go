package solana

import (
	"fmt"
	"reflect"
	"sync"
	"time"
)

type RetainedAccount struct {
	Account    *Account
	Slot       Slot
	ObservedAt time.Time
}

type FrozenAccountState struct {
	// ReceivedAt is the latest accepted local input observation.
	ReceivedAt time.Time
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

// SeedWindow replaces the retained address set while preserving equal-slot or newer live payloads.
// Previously frozen snapshots remain detached. The caller owns subscription reconfiguration.
//
// Version:
//   - 2026-09-11: Added for bounded array-window recovery.
func (s *AccountState) SeedWindow(accounts []*Account, slot Slot, observed time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[Address]RetainedAccount, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if previous, ok := s.accounts[account.Address]; ok && previous.Slot >= slot {
			next[account.Address] = previous
		} else {
			next[account.Address] = copyRetainedAccount(RetainedAccount{account, slot, observed})
		}
	}
	s.accounts = next
}

// Apply retains a full account notification, preserving its individual slot and time.
// Older and identical same-slot notifications preserve the original receipt.
// Different account contents at equal slots follow reception order.
//
// Version:
//   - 2026-09-10: Preserve accepted receipt time separately from oldest provenance.
//   - 2026-09-09: Added.
func (s *AccountState) Apply(update *AccountUpdate, observed time.Time) error {
	return s.apply(update, observed, false)
}

// ApplyReceived retains AMM account payloads in receipt order, including lower slots.
// Exact duplicate notifications preserve their original receipt time.
//
// Version:
//   - 2026-09-12: Added.
func (s *AccountState) ApplyReceived(update *AccountUpdate, observed time.Time) error {
	return s.apply(update, observed, true)
}

func (s *AccountState) apply(update *AccountUpdate, observed time.Time, receiptOrder bool) error {
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
	if previous, exists := s.accounts[update.Account.Address]; exists && ((!receiptOrder && previous.Slot > update.Slot) || (receiptOrder && observed.Before(previous.ObservedAt)) || (previous.Slot == update.Slot && reflect.DeepEqual(previous.Account, update.Account))) {
		return nil
	}
	s.accounts[update.Account.Address] = copyRetainedAccount(RetainedAccount{update.Account, update.Slot, observed})
	return nil
}

// Freeze copies all retained inputs without waiting for an RPC or matching slots.
// Slot and ObservedAt are conservative minima; ReceivedAt is the latest receipt.
// Per-account provenance is retained; ReceivedAt does not assert all inputs are fresh.
//
// Version:
//   - 2026-09-10: Preserve accepted receipt time separately from oldest provenance.
//   - 2026-09-09: Added.
func (s *AccountState) Freeze() FrozenAccountState {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := FrozenAccountState{Accounts: make(map[Address]RetainedAccount, len(s.accounts))}
	for address, account := range s.accounts {
		result.Accounts[address] = copyRetainedAccount(account)
		if account.ObservedAt.After(result.ReceivedAt) {
			result.ReceivedAt = account.ObservedAt
		}
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
