package dlmm

import (
	"context"
	"errors"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"slices"
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
	quoteSnapshotObserver func(*QuoteSnapshot)
	client                *Client
	pool                  solana.Address
	addresses             []solana.Address
	maxAge                time.Duration
	now                   func() time.Time
	refresh               sync.Mutex
	mu                    sync.Mutex
	generation            uint64
	updates               chan struct{}
	checkedKey            string
	minimumSlot           solana.Slot
	connected             int
	active                map[solana.Address]bool
	running               bool
	snapshot              snapshotAccounts
	changed               chan struct{}
	observedAt            time.Time
	retained              solana.AccountState
}

// NewStateCache creates a bounded-age cache for a discovered Meteora DLMM pool.
// Run enables notification-based reuse; without Run each quote refreshes through RPC.
//
// Version:
//   - 2026-09-07: Added.
func NewStateCache(client *Client, pool solana.Address, maxAge time.Duration) (*StateCache, error) {
	if client == nil || client.snapshots == nil {
		return nil, fmt.Errorf("failed to create meteora dlmm state cache: client=null")
	}
	_, ok := client.pools[pool]
	if !ok {
		return nil, fmt.Errorf("failed to create meteora dlmm state cache: pool=invalid")
	}
	if maxAge <= 0 {
		return nil, fmt.Errorf("failed to create meteora dlmm state cache: max_age=out_of_range")
	}
	return &StateCache{client: client, pool: pool, addresses: []solana.Address{pool}, maxAge: maxAge, now: time.Now, changed: make(chan struct{}, 1), active: make(map[solana.Address]bool)}, nil
}

// Run maintains account streams until cancellation or a stream failure.
// Full payload receivers also update the independent retained calculation inputs.
// A failure invalidates the coherent cache and is returned so the owner can reconnect.
//
// Version:
//   - 2026-09-09: Preserve bootstrap inputs across subscription replacement; clear on session exit.
//   - 2026-09-09: Withdraw calculation snapshots when an account subscription disconnects.
//   - 2026-09-09: Retain full account updates for local snapshot quotes.
//   - 2026-09-07: Added.
func (s *StateCache) Run(ctx context.Context, subscribe SubscribeAccountChangesFunc) error {
	if s == nil || ctx == nil || subscribe == nil {
		return fmt.Errorf("failed to run meteora dlmm state cache: dependency=null")
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("failed to run meteora dlmm state cache: running=true")
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running = false
		s.retained.Reset()
		s.publishQuoteSnapshotLocked()
	}()
	for ctx.Err() == nil {
		s.mu.Lock()
		addresses := append([]solana.Address(nil), s.addresses...)
		// This subscription set already includes changes queued before capture.
		select {
		case <-s.changed:
		default:
		}
		s.mu.Unlock()
		session, cancel := context.WithCancel(ctx)
		results := make(chan error, len(addresses))
		for _, address := range addresses {
			go func() { results <- s.watch(session, address, subscribe) }()
		}
		reconfigure := false
		var failures []error
		remaining := len(addresses)
		select {
		case <-ctx.Done():
		case <-s.changed:
			reconfigure = true
		case err := <-results:
			remaining--
			if err != nil && !errors.Is(err, context.Canceled) {
				failures = append(failures, err)
			}
		}
		cancel()
		for ; remaining > 0; remaining-- {
			err := <-results
			if err != nil && !errors.Is(err, context.Canceled) {
				failures = append(failures, err)
			}
		}
		if len(failures) > 0 {
			return errors.Join(failures...)
		}
		if !reconfigure {
			return nil
		}
	}
	return nil
}

func (s *StateCache) watch(ctx context.Context, address solana.Address, subscribe SubscribeAccountChangesFunc) error {
	sub, err := subscribe(address)
	if err != nil {
		return fmt.Errorf("failed to subscribe meteora dlmm state: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("failed to subscribe meteora dlmm state: subscription=null")
	}
	s.mu.Lock()
	s.connected++
	s.active[address] = true
	s.invalidate(0)
	s.mu.Unlock()
	defer func() {
		sub.Close()
		s.mu.Lock()
		s.connected--
		delete(s.active, address)
		s.invalidate(0)
		s.mu.Unlock()
	}()
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
			return fmt.Errorf("failed to receive meteora dlmm state: %w", err)
		}
		if err := slot.Validate(); err != nil {
			return fmt.Errorf("failed to receive meteora dlmm state: %w", err)
		}
		s.mu.Lock()
		s.invalidate(slot)
		s.mu.Unlock()
	}
}
func (s *StateCache) invalidate(slot solana.Slot) {
	defer s.publishQuoteSnapshotLocked()
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
//   - 2026-09-09: Publish initialized account inputs to the snapshot observer.
//   - 2026-09-09: Classify state-change retries separately from transport failures.
//   - 2026-09-07: Added.
func (s *StateCache) QuoteExactInputs(ctx context.Context, requests []ExactInputRequest) (CachedQuote, error) {
	if s == nil {
		return CachedQuote{}, fmt.Errorf("failed to quote cached meteora dlmm state: cache=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote cached meteora dlmm state: %w", err)
	}
	if len(requests) == 0 {
		return CachedQuote{}, fmt.Errorf("failed to quote cached meteora dlmm state: requests=empty")
	}
	s.refresh.Lock()
	defer s.refresh.Unlock()
	if err := ctx.Err(); err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote cached meteora dlmm state: %w", err)
	}
	s.mu.Lock()
	snapshot, observed := s.snapshot, s.observedAt
	generation, minimum := s.generation, s.minimumSlot
	valid := snapshot != nil && s.connected == len(s.addresses) && s.now().Sub(observed) < s.maxAge
	for _, address := range s.addresses {
		if !s.active[address] {
			valid = false
			break
		}
	}
	s.mu.Unlock()
	var result CachedQuote
	var addresses []solana.Address
	var refreshedPool any
	if valid {
		local := &Client{accounts: snapshot, programID: s.client.programID, pools: s.client.pools}
		result = CachedQuote{QuoteBatch: QuoteBatch{Slot: minimum, Quotes: make([]Quote, len(requests))}, ObservedAt: observed}
		for i, r := range requests {
			q, err := local.quoteExactInput(ctx, s.pool, r.InputMint, r.AmountIn)
			if err != nil {
				var missing *solana.QuoteArrayRequiredError
				if !errors.As(err, &missing) {
					return CachedQuote{}, fmt.Errorf("failed to quote cached meteora dlmm state: %w", err)
				}
				valid = false
				break
			}
			result.Quotes[i] = q
		}
	}
	if !valid {
		local := &Client{initialArrayCount: s.client.initialArrayCount, maxArrayCount: s.client.maxArrayCount, accounts: s.client.accounts, programID: s.client.programID, pools: s.client.pools}
		if seed, ok := s.client.quotePools.Load(s.pool); ok {
			local.quotePools.Store(s.pool, seed)
		}
		capture := &cacheSnapshotProvider{source: s.client.snapshots, minimum: minimum}
		local.snapshots = capture
		batch, err := local.QuoteExactInputsWithSlot(ctx, s.pool, requests)
		if err != nil {
			return CachedQuote{}, fmt.Errorf("failed to refresh meteora dlmm state: %w", err)
		}
		snapshot, addresses = capture.accounts, capture.addresses
		refreshedPool, _ = local.quotePools.Load(s.pool)
		observed = s.now()
		result = CachedQuote{QuoteBatch: batch, ObservedAt: observed}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation {
		return CachedQuote{}, fmt.Errorf("failed to quote cached meteora dlmm state: %w: state changed during calculation", quotestate.ErrStateChanged)
	}
	s.snapshot, s.observedAt = snapshot, observed
	seedAccounts := make([]*solana.Account, 0, len(snapshot))
	for _, account := range snapshot {
		seedAccounts = append(seedAccounts, account)
	}
	s.retained.Seed(seedAccounts, result.Slot, observed)
	s.publishQuoteSnapshotLocked()
	if refreshedPool != nil {
		s.client.quotePools.Store(s.pool, refreshedPool)
	}
	if result.Slot > s.minimumSlot {
		s.minimumSlot = result.Slot
	}
	if addresses != nil && !sameAddresses(s.addresses, addresses) {
		s.addresses = addresses
		select {
		case s.changed <- struct{}{}:
		default:
		}
	}
	return result, nil
}

func sameAddresses(a, b []solana.Address) bool {
	if len(a) != len(b) {
		return false
	}
	for _, v := range a {
		if !slices.Contains(b, v) {
			return false
		}
	}
	return true
}

type cacheSnapshotProvider struct {
	source    accountSnapshotProvider
	minimum   solana.Slot
	addresses []solana.Address
	accounts  snapshotAccounts
}

// AccountSnapshot captures detached pool, Clock and adaptive bin-array state.
//
// Version:
//   - 2026-09-07: Added.
func (p *cacheSnapshotProvider) AccountSnapshot(ctx context.Context, requested []solana.Address) (*solana.AccountSnapshot, error) {
	addresses := append([]solana.Address(nil), requested...)
	value, err := p.source.AccountSnapshot(ctx, addresses)
	if err != nil {
		return nil, fmt.Errorf("failed to capture meteora dlmm state: %w", err)
	}
	if value == nil || len(value.Accounts) != len(addresses) {
		return nil, fmt.Errorf("failed to capture meteora dlmm state: snapshot=invalid")
	}
	if err := value.Slot.Validate(); err != nil {
		return nil, fmt.Errorf("failed to capture meteora dlmm state: %w", err)
	}
	if value.Slot < p.minimum {
		return nil, fmt.Errorf("failed to capture meteora dlmm state: snapshot behind observed state: snapshot_slot=%d minimum_slot=%d", value.Slot, p.minimum)
	}
	detached := make([]*solana.Account, len(addresses))
	accounts := make(snapshotAccounts, len(addresses))
	for i, a := range value.Accounts {
		if a == nil {
			return nil, fmt.Errorf("failed to capture meteora dlmm state: account=null")
		}
		copyAccount := *a
		copyAccount.Data = append([]byte(nil), a.Data...)
		detached[i] = &copyAccount
		accounts[addresses[i]] = &copyAccount
	}
	p.minimum = value.Slot
	// Retain Clock notifications as calculation inputs, without triggering quotes.
	p.addresses = addresses
	p.accounts = accounts
	return &solana.AccountSnapshot{Slot: value.Slot, Accounts: detached[:len(requested)]}, nil
}
