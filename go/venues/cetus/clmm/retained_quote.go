package clmm

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type retainedSnapshot struct {
	snapshot *LocalSnapshot
	handle   sui.Address
	baseline sui.Checkpoint
	version  uint64
	digest   sui.ObjectDigest
}

type ObjectStateSubscriber interface {
	SubscribeObjectState(context.Context, sui.Address) (*sui.TransactionSubscription, error)
}

// RunRetained subscribes before initialization and retains full pool/tick updates.
// Disconnects discard state. The caller reconnects and initializes again.
//
// Version:
//   - 2026-09-09: Publish retained input updates and withdraw unavailable state.
//   - 2026-09-09: Added.
func (c *StateCache) RunRetained(ctx context.Context, subscriber ObjectStateSubscriber) error {
	if c == nil || ctx == nil || subscriber == nil {
		return fmt.Errorf("failed to run retained cetus state: dependency=null")
	}
	c.retainedMu.Lock()
	if c.retainedRunning {
		c.retainedMu.Unlock()
		return fmt.Errorf("failed to run retained cetus state: subscription=active")
	}
	c.retainedRunning = true
	c.retained = nil
	c.publishQuoteSnapshotLocked()
	c.retainedMu.Unlock()
	defer func() {
		c.retainedMu.Lock()
		c.retainedRunning = false
		c.retained = nil
		c.publishQuoteSnapshotLocked()
		c.retainedMu.Unlock()
	}()
	sub, err := subscriber.SubscribeObjectState(ctx, c.pool)
	if err != nil {
		return fmt.Errorf("failed to run retained cetus state: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("failed to run retained cetus state: subscription=null")
	}
	defer sub.Close()

	warmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err = c.Warm(warmCtx)
	cancel()
	if err != nil {
		return err
	}
	for {
		n, err := sub.Recv(ctx)
		if err != nil {
			return fmt.Errorf("failed to receive retained cetus state: %w", err)
		}
		if n.Effects == nil {
			continue
		}
		if err := c.applyRetainedObjects(n); err != nil {
			return err
		}
	}
}

// QuoteRetainedPair freezes current inputs and calculates without network reads or Trade-position checks.
// Checkpoint and timestamp retain the initialization baseline of the tick set;
// the pool version may be newer. This does not assert an atomic on-chain snapshot.
//
// Version:
//   - 2026-09-09: Keep local capture time separate from input provenance.
func (c *StateCache) QuoteRetainedPair(ctx context.Context, p QuotePairParams) (QuotePairResult, error) {
	if c == nil || ctx == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained cetus state: dependency=null")
	}
	if p.Bid.Pool.Address != c.pool || p.Ask.Pool.Address != c.pool || p.Bid.A2B == p.Ask.A2B {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained cetus state: parameters=invalid")
	}
	if err := ctx.Err(); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained cetus state: %w", err)
	}
	c.retainedMu.Lock()
	if c.retained == nil {
		c.retainedMu.Unlock()
		return QuotePairResult{}, fmt.Errorf("failed to quote retained cetus state: snapshot=null")
	}
	capturedAt := time.Now()
	s := *c.retained
	s.snapshot = cloneLocalSnapshot(s.snapshot)
	c.retainedMu.Unlock()
	bid, err := s.snapshot.Quote(p.Bid.AmountIn, p.Bid.A2B, true)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained cetus bid: %w", err)
	}
	ask, err := s.snapshot.Quote(p.Ask.AmountOut, p.Ask.A2B, false)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained cetus ask: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained cetus state: %w", err)
	}
	bid.Checkpoint = s.baseline.SequenceNumber
	ask.Checkpoint = s.baseline.SequenceNumber
	return QuotePairResult{CapturedAt: capturedAt, Bid: bid, Ask: ask, Checkpoint: s.baseline.SequenceNumber, PoolVersion: s.version, PoolDigest: s.digest, StateTimestamp: s.baseline.Timestamp}, nil
}

func cloneLocalSnapshot(source *LocalSnapshot) *LocalSnapshot {
	s := *source
	clone := func(n *big.Int) *big.Int {
		if n == nil {
			return nil
		}
		return new(big.Int).Set(n)
	}
	s.Pool.CoinA = clone(s.Pool.CoinA)
	s.Pool.CoinB = clone(s.Pool.CoinB)
	s.Pool.CurrentSqrtPrice = clone(s.Pool.CurrentSqrtPrice)
	s.Pool.Liquidity = clone(s.Pool.Liquidity)
	s.Ticks = append([]Tick(nil), source.Ticks...)
	for i := range s.Ticks {
		s.Ticks[i].SqrtPrice = clone(s.Ticks[i].SqrtPrice)
		s.Ticks[i].LiquidityNet = clone(s.Ticks[i].LiquidityNet)
	}
	return &s
}

func retainedTickHandle(obj *sui.Object) (sui.Address, error) {
	if obj == nil || obj.Move == nil {
		return sui.Address{}, fmt.Errorf("failed to parse retained cetus pool: object=null")
	}
	var fields struct {
		Manager struct {
			Ticks struct {
				ID string `json:"id"`
			} `json:"ticks"`
		} `json:"tick_manager"`
	}
	if err := json.Unmarshal(obj.Move.JSON, &fields); err != nil {
		return sui.Address{}, fmt.Errorf("failed to parse retained cetus pool: %w", err)
	}
	id, err := sui.ParseAddress(fields.Manager.Ticks.ID)
	if err != nil {
		return sui.Address{}, fmt.Errorf("failed to parse retained cetus tick handle: %w", err)
	}
	return id, nil
}

func (c *StateCache) applyRetainedObjects(n *sui.TransactionNotification) (err error) {
	c.retainedMu.Lock()
	defer func() { c.publishQuoteSnapshotLocked(); c.retainedMu.Unlock() }()
	defer func() {
		if err != nil {
			c.retained = nil
		}
	}()
	if c.retained == nil || n == nil || n.Effects == nil || n.Effects.Checkpoint == nil {
		return fmt.Errorf("failed to apply retained cetus objects: state=null")
	}
	s := c.retained
	if *n.Effects.Checkpoint <= s.baseline.SequenceNumber {
		return nil
	}
	// The shared pool's version orders its transactions, including multiple
	// transactions in one checkpoint. Replays never roll retained state back.
	var poolChange *sui.ObjectChange
	for i := range n.ObjectChanges {
		if n.ObjectChanges[i].Address == c.pool {
			poolChange = &n.ObjectChanges[i]
			break
		}
	}
	if poolChange == nil {
		for _, change := range n.ObjectChanges {
			if change.InputParent == s.handle || change.OutputParent == s.handle {
				return fmt.Errorf("failed to apply retained cetus objects: pool_change=missing")
			}
		}
		return nil
	}
	if poolChange.After == nil {
		return fmt.Errorf("failed to apply retained cetus objects: pool=deleted")
	}
	if poolChange.After.Version < s.version {
		return nil
	}
	if poolChange.After.Version == s.version {
		if poolChange.After.Digest != s.digest {
			return fmt.Errorf("failed to apply retained cetus objects: digest=mismatch")
		}
		return nil
	}
	// Pool and field payloads are full replacements, not arithmetic deltas.
	// Keep the latest received components even when the input version differs
	// from our baseline. Trade quotes freeze these receiver-side snapshots;
	// they do not claim a transaction-atomic pool/tick state.
	p, err := ParsePool(poolChange.After)
	if err != nil {
		return fmt.Errorf("failed to apply retained cetus pool: %w", err)
	}
	handle, err := retainedTickHandle(poolChange.After)
	if err != nil {
		return err
	}
	if handle != s.handle || p.CoinTypeA != s.snapshot.Pool.CoinTypeA || p.CoinTypeB != s.snapshot.Pool.CoinTypeB {
		return fmt.Errorf("failed to apply retained cetus pool: configuration=changed")
	}
	for _, change := range n.ObjectChanges {
		if change.InputParent != s.handle && change.OutputParent != s.handle {
			continue
		}
		obj := change.After
		remove := change.Deleted || change.OutputParent != s.handle
		if remove {
			obj = change.Before
		}
		if obj == nil || obj.Move == nil {
			return fmt.Errorf("failed to apply retained cetus tick: object=null")
		}
		var field struct {
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(obj.Move.JSON, &field); err != nil {
			return fmt.Errorf("failed to decode retained cetus field: %w", err)
		}
		tick, err := parseTick(field.Value)
		if err != nil {
			return fmt.Errorf("failed to apply retained cetus tick: %w", err)
		}
		found := -1
		for i := range s.snapshot.Ticks {
			if s.snapshot.Ticks[i].Index == tick.Index {
				found = i
				break
			}
		}
		if remove {
			if found < 0 {
				return fmt.Errorf("failed to remove retained cetus tick: tick=unknown")
			}
			s.snapshot.Ticks = append(s.snapshot.Ticks[:found], s.snapshot.Ticks[found+1:]...)
		} else if found >= 0 {
			s.snapshot.Ticks[found] = tick
		} else {
			s.snapshot.Ticks = append(s.snapshot.Ticks, tick)
		}
	}
	if len(s.snapshot.Ticks) > 10000 {
		return fmt.Errorf("failed to apply retained cetus objects: tick_count=out_of_range")
	}
	s.snapshot.Pool = *p
	s.version = poolChange.After.Version
	s.digest = poolChange.After.Digest
	return nil
}
