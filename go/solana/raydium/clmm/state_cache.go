package clmm

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
	ReceivedAt time.Time
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
	configs               map[solana.Address]AMMConfig
	changed               chan struct{}
	observedAt            time.Time
	retained              solana.AccountState
}

// NewStateCache creates a bounded-age cache for a discovered CLMM pool.
// Run enables notification-based reuse; without Run each quote refreshes through RPC.
//
// Version:
//   - 2026-09-07: Added.
func NewStateCache(client *Client, pool solana.Address, maxAge time.Duration) (*StateCache, error) {
	if client == nil || client.snapshots == nil {
		return nil, fmt.Errorf("failed to create clmm state cache: client=null")
	}
	p, ok := client.pools[pool]
	if !ok {
		return nil, fmt.Errorf("failed to create clmm state cache: pool=invalid")
	}
	if maxAge <= 0 {
		return nil, fmt.Errorf("failed to create clmm state cache: max_age=out_of_range")
	}
	return &StateCache{client: client, pool: pool, addresses: []solana.Address{pool, p.AMMConfig}, maxAge: maxAge, now: time.Now, changed: make(chan struct{}, 1), active: make(map[solana.Address]bool)}, nil
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
		return fmt.Errorf("failed to run clmm state cache: dependency=null")
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("failed to run clmm state cache: running=true")
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
		return fmt.Errorf("failed to subscribe clmm state: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("failed to subscribe clmm state: subscription=null")
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
			return fmt.Errorf("failed to receive clmm state: %w", err)
		}
		if err := slot.Validate(); err != nil {
			return fmt.Errorf("failed to receive clmm state: %w", err)
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
		return CachedQuote{}, fmt.Errorf("failed to quote cached clmm state: cache=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote cached clmm state: %w", err)
	}
	if len(requests) == 0 {
		return CachedQuote{}, fmt.Errorf("failed to quote cached clmm state: requests=empty")
	}
	s.refresh.Lock()
	defer s.refresh.Unlock()
	if err := ctx.Err(); err != nil {
		return CachedQuote{}, fmt.Errorf("failed to quote cached clmm state: %w", err)
	}
	s.mu.Lock()
	snapshot, observed := s.snapshot, s.observedAt
	configs := s.configs
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
		local := &Client{accounts: snapshot, programID: s.client.programID, pools: s.client.pools, configs: configs}
		result = CachedQuote{QuoteBatch: QuoteBatch{Slot: minimum, Quotes: make([]Quote, len(requests))}, ObservedAt: observed}
		for i, r := range requests {
			q, err := local.quoteExactInput(ctx, s.pool, r.InputMint, r.AmountIn)
			if err != nil {
				var missing *solana.QuoteArrayRequiredError
				if !errors.As(err, &missing) {
					return CachedQuote{}, fmt.Errorf("failed to quote cached clmm state: %w", err)
				}
				valid = false
				break
			}
			result.Quotes[i] = q
		}
	}
	if !valid {
		local := &Client{initialArrayCount: s.client.initialArrayCount, maxArrayCount: s.client.maxArrayCount, accounts: s.client.accounts, programID: s.client.programID, pools: s.client.pools, configs: make(map[solana.Address]AMMConfig)}
		if seed, ok := s.client.quotePools.Load(s.pool); ok {
			local.quotePools.Store(s.pool, seed)
		}
		capture := &cacheSnapshotProvider{source: s.client.snapshots, local: local, pool: s.pool, minimum: minimum}
		local.snapshots = capture
		batch, err := local.QuoteExactInputsWithSlot(ctx, s.pool, requests)
		if err != nil {
			return CachedQuote{}, fmt.Errorf("failed to refresh clmm state: %w", err)
		}
		snapshot, configs, addresses = capture.accounts, local.configs, capture.addresses
		refreshedPool, _ = local.quotePools.Load(s.pool)
		observed = s.now()
		result = CachedQuote{QuoteBatch: batch, ObservedAt: observed}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation {
		return CachedQuote{}, fmt.Errorf("failed to quote cached clmm state: %w: state changed during calculation", quotestate.ErrStateChanged)
	}
	s.snapshot, s.configs, s.observedAt = snapshot, configs, observed
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
	local     *Client
	pool      solana.Address
	minimum   solana.Slot
	addresses []solana.Address
	accounts  snapshotAccounts
}

// AccountSnapshot captures detached pool, config and adaptive tick-array state.
//
// Version:
//   - 2026-09-07: Added.
func (p *cacheSnapshotProvider) AccountSnapshot(ctx context.Context, requested []solana.Address) (*solana.AccountSnapshot, error) {
	configured := p.local.pools[p.pool]
	addresses := append(append([]solana.Address(nil), requested...), configured.AMMConfig)
	value, err := p.source.AccountSnapshot(ctx, addresses)
	if err != nil {
		return nil, fmt.Errorf("failed to capture clmm state: %w", err)
	}
	if value == nil || len(value.Accounts) != len(addresses) {
		return nil, fmt.Errorf("failed to capture clmm state: snapshot=invalid")
	}
	if err := value.Slot.Validate(); err != nil {
		return nil, fmt.Errorf("failed to capture clmm state: %w", err)
	}
	if value.Slot < p.minimum {
		return nil, fmt.Errorf("failed to capture clmm state: snapshot behind observed state: snapshot_slot=%d minimum_slot=%d", value.Slot, p.minimum)
	}
	detached := make([]*solana.Account, len(addresses))
	accounts := make(snapshotAccounts, len(addresses))
	for i, a := range value.Accounts {
		if a == nil {
			return nil, fmt.Errorf("failed to capture clmm state: account=null")
		}
		copyAccount := *a
		copyAccount.Data = append([]byte(nil), a.Data...)
		detached[i] = &copyAccount
		accounts[addresses[i]] = &copyAccount
	}
	configAccount := detached[len(detached)-1]
	if configAccount.Owner != p.local.programID {
		return nil, fmt.Errorf("failed to capture clmm state: config_owner=invalid")
	}
	config, err := decodeAMMConfig(configured.AMMConfig, configAccount.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to capture clmm state: %w", err)
	}
	poolAccount := accounts[p.pool]
	if poolAccount.Owner != p.local.programID {
		return nil, fmt.Errorf("failed to capture clmm state: pool_owner=invalid")
	}
	pool, err := decodePool(p.pool, poolAccount.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to capture clmm state: %w", err)
	}
	if config.TickSpacing != pool.TickSpacing {
		return nil, fmt.Errorf("failed to capture clmm state: tick_spacing=invalid")
	}
	p.local.configs[config.Address] = config
	p.minimum = value.Slot
	p.addresses, p.accounts = addresses, accounts
	return &solana.AccountSnapshot{Slot: value.Slot, Accounts: detached[:len(requested)]}, nil
}
