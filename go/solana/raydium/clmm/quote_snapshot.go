package clmm

import (
	"context"
	"fmt"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"maps"
	"time"
)

type QuoteSnapshot struct {
	accounts  solana.FrozenAccountState
	programID solana.Address
	pool      solana.Address
	pools     map[solana.Address]Pool
}

// QuoteExactInputs calculates only from detached account inputs.
//
// Version:
//   - 2026-09-09: Added.
func (s *QuoteSnapshot) QuoteExactInputs(ctx context.Context, requests []ExactInputRequest) (CachedQuote, error) {
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
	frozen := s.accounts
	if len(frozen.Accounts) == 0 {
		return CachedQuote{}, fmt.Errorf("failed to quote retained state: snapshot=null")
	}
	accounts := make(snapshotAccounts, len(frozen.Accounts))
	for address, value := range frozen.Accounts {
		accounts[address] = value.Account
	}
	local := &Client{accounts: accounts, programID: s.programID, pools: s.pools}

	configured := s.pools[s.pool]
	account := accounts[configured.AMMConfig]
	if account == nil || account.Owner != s.programID {
		return CachedQuote{}, fmt.Errorf("failed to quote retained clmm state: config=invalid")
	}
	config, err := decodeAMMConfig(configured.AMMConfig, account.Data)
	if err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote retained clmm state: %w", err)
	}
	local.configs = map[solana.Address]AMMConfig{configured.AMMConfig: config}

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

// SetQuoteSnapshotObserver installs a serialized consumer of retained account updates.
// The callback must not call back into this cache. Nil withdraws disconnected state.
//
// Version:
//   - 2026-09-09: Added.
func (s *StateCache) SetQuoteSnapshotObserver(observer func(*QuoteSnapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quoteSnapshotObserver = observer
	s.publishQuoteSnapshotLocked()
}
func (s *StateCache) publishQuoteSnapshotLocked() {
	if s.quoteSnapshotObserver == nil {
		return
	}
	if s.connected != len(s.addresses) {
		s.quoteSnapshotObserver(nil)
		return
	}
	frozen := s.retained.Freeze()
	if len(frozen.Accounts) == 0 {
		s.quoteSnapshotObserver(nil)
		return
	}
	s.quoteSnapshotObserver(&QuoteSnapshot{accounts: frozen, programID: s.client.programID, pool: s.pool, pools: maps.Clone(s.client.pools)})
}
