package spot

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/internal/suiwindow"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"math/big"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type ObjectStateSubscriber interface {
	SubscribeObjectState(context.Context, sui.Address) (*sui.TransactionSubscription, error)
}

// RunRetained subscribes before initializing and applies full objects without publishing quotes.
// Initialization may acquire state; Trade calculation never invokes it.
//
// Version:
//   - 2026-09-12: Retain inputs across transport reconnects without refreshing their receipt time.
//   - 2026-09-11: Refill ranges asynchronously while applying live state and replay before installation.
//   - 2026-09-11: Reacquire missing reference coverage in the state producer.
//   - 2026-09-10: Preserve input receipt time.
//   - 2026-09-09: Publish retained input updates and withdraw unavailable state.
//   - 2026-09-09: Added.
func (c *StateCache) RunRetained(ctx context.Context, subscribed ObjectStateSubscriber, initialize func(context.Context) error) error {
	if c == nil || ctx == nil || subscribed == nil || initialize == nil {
		return fmt.Errorf("failed to retain bluefin state: dependency=null")
	}
	c.retainedMu.Lock()
	if c.retainedRunning {
		c.retainedMu.Unlock()
		return fmt.Errorf("failed to retain bluefin state: subscription=active")
	}
	c.retainedRunning = true
	needsInit := c.retained == nil
	c.publishQuoteSnapshotLocked()
	c.retainedMu.Unlock()
	defer func() {
		c.retainedMu.Lock()
		c.retainedRunning = false
		c.publishQuoteSnapshotLocked()
		c.retainedMu.Unlock()
	}()
	sub, err := subscribed.SubscribeObjectState(ctx, c.pool)
	if err != nil {
		return fmt.Errorf("failed to subscribe retained bluefin state: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("failed to subscribe retained bluefin state: subscription=null")
	}
	defer sub.Close()
	if needsInit {
		initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = initialize(initCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("failed to initialize retained bluefin state: %w", err)
		}
	}
	return suiwindow.Run(ctx, sub.Recv, func(update suiwindow.Update) error {
		if err := c.applyRetainedObjects(update.Notification, update.ReceivedAt); err != nil {
			if cp := update.Notification.Effects.Checkpoint; cp != nil {
				c.ObserveCheckpoint(*cp)
			}
			return err
		}
		return nil
	}, func() bool { return c.retainedCoverageMissing(ctx) }, c.captureRetainedWindow, c.installRetainedWindow)
}

// QuoteRetainedPair calculates from one detached set of retained inputs with no position wait or RPC.
// Checkpoint/time describe the initial verified input baseline; pool version may be newer.
//
// Version:
//   - 2026-09-10: Preserve input receipt time.
//   - 2026-09-09: Keep local capture time separate from input provenance.
func (c *StateCache) QuoteRetainedPair(ctx context.Context, p QuotePairParams) (QuotePairResult, error) {
	if c == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained bluefin state: cache=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if p.Bid.Pool.Address != c.pool || p.Ask.Pool.Address != c.pool || p.Bid.A2B == p.Ask.A2B {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained bluefin state: parameters=invalid")
	}
	if err := ctx.Err(); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained bluefin state: %w", err)
	}
	c.retainedMu.Lock()
	capturedAt := time.Now()
	s := cloneRetainedState(c.retained)
	head := c.retainedHead
	c.retainedMu.Unlock()
	if s == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained bluefin state: snapshot=null")
	}

	bid, err := c.quote(ctx, s, p.Bid.AmountIn, p.Bid.A2B, true, p.Bid.SqrtPriceLimit)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained bluefin bid: %w", err)
	}
	ask, err := c.quote(ctx, s, p.Ask.AmountOut, p.Ask.A2B, false, p.Ask.SqrtPriceLimit)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained bluefin ask: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote retained bluefin state: %w", err)
	}
	bid.Checkpoint = head.SequenceNumber
	ask.Checkpoint = head.SequenceNumber
	return QuotePairResult{ReceivedAt: s.received, CapturedAt: capturedAt, Bid: bid, Ask: ask, Checkpoint: head.SequenceNumber, PoolVersion: s.version, PoolDigest: s.digest, StateTimestamp: head.Timestamp}, nil
}
func cloneRetainedState(source *quoteState) *quoteState {
	if source == nil {
		return nil
	}
	s := *source
	s.pool.SqrtPrice = new(big.Int).Set(source.pool.SqrtPrice)
	s.pool.Liquidity = new(big.Int).Set(source.pool.Liquidity)
	s.words = make(map[int32]*big.Int, len(source.words))
	s.nets = make(map[int32]*big.Int, len(source.nets))
	for k, v := range source.words {
		s.words[k] = new(big.Int).Set(v)
	}
	for k, v := range source.nets {
		s.nets[k] = new(big.Int).Set(v)
	}
	return &s
}
func (c *StateCache) applyRetainedObjects(n *sui.TransactionNotification, receipt ...time.Time) (err error) {
	c.retainedMu.Lock()
	defer func() { c.publishQuoteSnapshotLocked(); c.retainedMu.Unlock() }()
	defer func() {
		if err != nil {
			c.retained = nil
		}
	}()
	old := c.retained
	if old == nil || n == nil || n.Effects == nil || n.Effects.Checkpoint == nil {
		return fmt.Errorf("failed to apply retained bluefin objects: state=null")
	}
	if *n.Effects.Checkpoint <= c.retainedHead.SequenceNumber {
		return nil
	}
	var changed *sui.ObjectChange
	for i := range n.ObjectChanges {
		if n.ObjectChanges[i].Address == c.pool {
			changed = &n.ObjectChanges[i]
			break
		}
	}
	if changed == nil {
		for _, v := range n.ObjectChanges {
			if v.InputParent == old.bitmap || v.OutputParent == old.bitmap || v.InputParent == old.ticks || v.OutputParent == old.ticks || v.InputParent == c.pool || v.OutputParent == c.pool {
				return fmt.Errorf("failed to apply retained bluefin objects: pool_change=missing")
			}
		}
		return nil
	}
	if changed.After == nil {
		return fmt.Errorf("failed to apply retained bluefin objects: pool=deleted")
	}
	if changed.After.Version < old.version {
		return nil
	}
	if changed.After.Version == old.version {
		if changed.After.Digest != old.digest {
			return fmt.Errorf("failed to apply retained bluefin objects: digest=mismatch")
		}
		return nil
	}
	// Pool and field payloads are full replacements, not arithmetic deltas.
	// Keep the latest received components even when the input version differs
	// from our baseline. Trade quotes freeze these receiver-side snapshots;
	// they do not claim a transaction-atomic pool/tick state.
	next, err := capturePool(changed.After, old.checkpoint)
	if err != nil {
		return fmt.Errorf("failed to apply retained bluefin pool: %w", err)
	}
	if next.bitmap != old.bitmap || next.ticks != old.ticks || next.keyType != old.keyType || next.pool.CoinTypeA != old.pool.CoinTypeA || next.pool.CoinTypeB != old.pool.CoinTypeB {
		return fmt.Errorf("failed to apply retained bluefin pool: configuration=changed")
	}
	next.words = old.words
	next.nets = old.nets
	next.retainedOnly = true
	next.captured = old.captured
	next.received = old.received
	for _, change := range n.ObjectChanges {
		parent := change.OutputParent
		obj := change.After
		remove := change.Deleted || ((change.InputParent == old.bitmap || change.InputParent == old.ticks) && change.OutputParent != change.InputParent)
		if remove {
			parent = change.InputParent
			obj = change.Before
		}
		if parent != old.bitmap && parent != old.ticks {
			continue
		}
		if obj == nil || obj.Move == nil {
			return fmt.Errorf("failed to apply retained bluefin field: object=null")
		}
		var field struct {
			Name struct {
				Bits json.RawMessage `json:"bits"`
			} `json:"name"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(obj.Move.JSON, &field); err != nil {
			return fmt.Errorf("failed to decode retained bluefin field: %w", err)
		}
		index, err := jsonUnsigned(field.Name.Bits)
		if err != nil {
			return fmt.Errorf("failed to decode retained bluefin key: %w", err)
		}
		if index.BitLen() > 32 {
			return fmt.Errorf("failed to decode retained bluefin key: index=out_of_range")
		}
		key := int32(uint32(index.Uint64()))
		if parent == old.bitmap {
			bits := new(big.Int)
			if !remove {
				bits, err = jsonUnsigned(field.Value)
				if err != nil {
					return fmt.Errorf("failed to decode retained bluefin bitmap: %w", err)
				}
			}
			if bits.BitLen() > 256 {
				return fmt.Errorf("failed to decode retained bluefin bitmap: word=out_of_range")
			}
			next.words[key] = bits
		} else {
			if remove {
				delete(next.nets, key)
				continue
			}
			var value struct {
				Net struct {
					Bits json.RawMessage `json:"bits"`
				} `json:"liquidity_net"`
			}
			if err := json.Unmarshal(field.Value, &value); err != nil {
				return fmt.Errorf("failed to decode retained bluefin tick: %w", err)
			}
			net, err := jsonUnsigned(value.Net.Bits)
			if err != nil {
				return fmt.Errorf("failed to decode retained bluefin net: %w", err)
			}
			if net.BitLen() > 128 {
				return fmt.Errorf("failed to decode retained bluefin net: liquidity=out_of_range")
			}
			if net.Bit(127) != 0 {
				net.Sub(net, new(big.Int).Lsh(big.NewInt(1), 128))
			}
			next.nets[key] = net
		}
	}
	position := quotestate.Position{Kind: "transaction", Sequence: n.Effects.Checkpoint.Uint64(), Digest: n.Effects.Digest.String()}
	if n.Effects.TransactionIndex != nil {
		index := *n.Effects.TransactionIndex
		position.Index = &index
	}
	next.position = &position
	at := time.Now().UTC()
	if len(receipt) > 0 {
		at = receipt[0]
	}
	if at.After(next.received) {
		next.received = at
	}
	c.retained = next
	return nil
}

var errRetainedCoverage = suiwindow.ErrCoverage
