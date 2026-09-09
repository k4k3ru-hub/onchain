package cpmm

import (
	"context"
	"errors"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"sync"
	"time"

	solana "github.com/k4k3ru-hub/onchain/go/solana"
)

type AccountChanges interface {
	Recv(context.Context) (solana.Slot, error)
	Close()
}
type SubscribeAccountChangesFunc func(solana.Address) (AccountChanges, error)
type CachedQuote struct {
	// CapturedAt is the local retained-input capture time, not an on-chain confirmation.
	CapturedAt time.Time
	QuoteBatch
	ObservedAt time.Time
}

// StateCache supports coherent RPC quotes and independent receiver-side retained quotes.
// Retained account streams do not assert cross-account atomicity.
type StateCache struct {
	client      *Client
	pool        solana.Address
	addresses   []solana.Address
	maxAge      time.Duration
	now         func() time.Time
	refresh     sync.Mutex
	mu          sync.Mutex
	generation  uint64
	updates     chan struct{}
	checkedKey  string
	minimumSlot solana.Slot
	connected   int
	running     bool
	snapshot    *solana.AccountSnapshot
	observedAt  time.Time
	retained    solana.AccountState
}

// NewStateCache creates a bounded-age cache for a discovered CPMM pool.
// Run enables notification-based reuse; without Run each quote refreshes through RPC.
//
// Version:
//   - 2026-09-07: Added.
func NewStateCache(client *Client, pool solana.Address, maxAge time.Duration) (*StateCache, error) {
	if client == nil || client.snapshots == nil {
		return nil, fmt.Errorf("failed to create cpmm state cache: client=null")
	}
	p, ok := client.pools[pool]
	if !ok {
		return nil, fmt.Errorf("failed to create cpmm state cache: pool=invalid")
	}
	if maxAge <= 0 {
		return nil, fmt.Errorf("failed to create cpmm state cache: max_age=out_of_range")
	}
	return &StateCache{client: client, pool: pool, addresses: []solana.Address{pool, p.AMMConfig, p.Token0Vault, p.Token1Vault}, maxAge: maxAge, now: time.Now}, nil
}

// Run maintains account streams until cancellation or a stream failure.
// Full payload receivers also update the independent retained calculation inputs.
// A failure invalidates the coherent cache and is returned so the owner can reconnect.
//
// Version:
//   - 2026-09-09: Retain full account updates for local snapshot quotes.
//   - 2026-09-07: Added.
func (s *StateCache) Run(ctx context.Context, subscribe SubscribeAccountChangesFunc) error {
	if s == nil || ctx == nil || subscribe == nil {
		return fmt.Errorf("failed to run cpmm state cache: dependency=null")
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("failed to run cpmm state cache: running=true")
	}
	s.running = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock() }()
	session, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, len(s.addresses))
	for _, address := range s.addresses {
		go func() { results <- s.watch(session, address, subscribe) }()
	}
	var failures []error
	for range s.addresses {
		err := <-results
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
func (s *StateCache) watch(ctx context.Context, address solana.Address, subscribe SubscribeAccountChangesFunc) error {
	sub, err := subscribe(address)
	if err != nil {
		return fmt.Errorf("failed to subscribe cpmm state: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("failed to subscribe cpmm state: subscription=null")
	}
	s.mu.Lock()
	s.connected++
	s.invalidate(0)
	s.mu.Unlock()
	defer func() { sub.Close(); s.mu.Lock(); s.connected--; s.invalidate(0); s.mu.Unlock() }()
	if stream, ok := sub.(interface {
		RecvState(context.Context) (*solana.AccountUpdate, error)
	}); ok {
		for {
			update, err := stream.RecvState(ctx)
			if err != nil {
				return fmt.Errorf("failed to receive retained account state: %w", err)
			}
			if update == nil || update.Account == nil || update.Account.Address != address {
				return fmt.Errorf("failed to receive retained account state: account=invalid")
			}
			if err := s.retained.Apply(update, s.now()); err != nil {
				return fmt.Errorf("failed to retain account state: %w", err)
			}
			s.mu.Lock()
			s.invalidate(update.Slot)
			s.mu.Unlock()
		}
	}
	for {
		slot, err := sub.Recv(ctx)
		if err != nil {
			return fmt.Errorf("failed to receive cpmm state: %w", err)
		}
		if err := slot.Validate(); err != nil {
			return fmt.Errorf("failed to receive cpmm state: %w", err)
		}
		s.mu.Lock()
		s.invalidate(slot)
		s.mu.Unlock()
	}
}
func (s *StateCache) invalidate(slot solana.Slot) {
	s.generation++
	s.signalChange()
	s.snapshot = nil
	if slot > s.minimumSlot {
		s.minimumSlot = slot
	}
}

// QuoteExactInputs computes quotes locally from a coherent cached account snapshot.
// Refresh is coalesced and required after changes, expiry or subscription loss.
// A notification arriving during refresh rejects that refresh for this call.
//
// Version:
//   - 2026-09-09: Classify state-change retries separately from transport failures.
//   - 2026-09-07: Added.
func (s *StateCache) QuoteExactInputs(ctx context.Context, requests []ExactInputRequest) (CachedQuote, error) {
	if s == nil {
		return CachedQuote{}, fmt.Errorf("failed to quote cached cpmm state: cache=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote cached cpmm state: %w", err)
	}
	if len(requests) == 0 {
		return CachedQuote{}, fmt.Errorf("failed to quote cached cpmm state: requests=empty")
	}
	s.refresh.Lock()
	defer s.refresh.Unlock()
	if err := ctx.Err(); err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote cached cpmm state: %w", err)
	}
	s.mu.Lock()
	snapshot, observed := s.snapshot, s.observedAt
	generation, minimum := s.generation, s.minimumSlot
	valid := snapshot != nil && s.connected == len(s.addresses) && s.now().Sub(observed) < s.maxAge
	s.mu.Unlock()
	if !valid {
		fetched, err := s.client.snapshots.AccountSnapshot(ctx, s.addresses)
		if err != nil {
			return CachedQuote{}, fmt.Errorf("failed to refresh cpmm state: %w", err)
		}
		if fetched == nil || len(fetched.Accounts) != len(s.addresses) {
			return CachedQuote{}, fmt.Errorf("failed to refresh cpmm state: snapshot=invalid")
		}
		if err := fetched.Slot.Validate(); err != nil {
			return CachedQuote{}, fmt.Errorf("failed to refresh cpmm state: %w", err)
		}
		if fetched.Slot < minimum {
			return CachedQuote{}, fmt.Errorf("failed to refresh cpmm state: snapshot behind observed state: snapshot_slot=%d minimum_slot=%d", fetched.Slot, minimum)
		}
		snapshot = &solana.AccountSnapshot{Slot: fetched.Slot, Accounts: make([]*solana.Account, len(fetched.Accounts))}
		for i, a := range fetched.Accounts {
			if a == nil {
				return CachedQuote{}, fmt.Errorf("failed to refresh cpmm state: account=null")
			}
			copied := *a
			copied.Data = append([]byte(nil), a.Data...)
			snapshot.Accounts[i] = &copied
		}
		observed = s.now()
	}
	accounts, err := newSnapshotAccounts(s.addresses, snapshot)
	if err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote cached cpmm state: %w", err)
	}
	local := &Client{accounts: accounts, programID: s.client.programID, pools: s.client.pools}
	result := CachedQuote{QuoteBatch: QuoteBatch{Slot: snapshot.Slot, Quotes: make([]Quote, len(requests))}, ObservedAt: observed}
	for i, r := range requests {
		q, err := local.quoteExactInput(ctx, s.pool, r.InputMint, r.AmountIn)
		if err != nil {
			return CachedQuote{}, fmt.Errorf("failed to quote cached cpmm state: %w", err)
		}
		result.Quotes[i] = q
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation {
		return CachedQuote{}, fmt.Errorf("failed to quote cached cpmm state: %w: state changed during calculation", quotestate.ErrStateChanged)
	}
	s.snapshot = snapshot
	s.retained.Seed(snapshot.Accounts, snapshot.Slot, observed)
	if snapshot.Slot > s.minimumSlot {
		s.minimumSlot = snapshot.Slot
	}
	s.observedAt = observed
	return result, nil
}
