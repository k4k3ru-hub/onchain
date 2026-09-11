package clmm

import (
	"context"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/internal/suiwindow"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"math/big"
)

func (c *StateCache) captureWindow(ctx context.Context, s *quoteState) error {
	words, nets, err := suiwindow.Read(ctx, c.reader, s.checkpoint, s.bitmap, s.ticks, s.keyType, s.pool.TickCurrentIndex, int32(s.pool.TickSpacing), jsonUnsigned)
	if err != nil {
		return fmt.Errorf("failed to capture turbos tick window: %w", err)
	}
	s.words, s.nets = words, nets
	return nil
}

func cloneReference(p QuotePairParams) QuotePairParams {
	if p.Bid.SqrtPriceLimit != nil {
		p.Bid.SqrtPriceLimit = new(big.Int).Set(p.Bid.SqrtPriceLimit)
	}
	if p.Ask.SqrtPriceLimit != nil {
		p.Ask.SqrtPriceLimit = new(big.Int).Set(p.Ask.SqrtPriceLimit)
	}
	if p.Bid.AmountIn != nil {
		p.Bid.AmountIn = new(big.Int).Set(p.Bid.AmountIn)
	}
	if p.Ask.AmountOut != nil {
		p.Ask.AmountOut = new(big.Int).Set(p.Ask.AmountOut)
	}
	return p
}

func (c *StateCache) retainedCoverageMissing(ctx context.Context) bool {
	c.retainedMu.Lock()
	s := c.retained
	if s == nil {
		c.retainedMu.Unlock()
		return false
	}
	missing := suiwindow.Missing(s.pool.TickCurrentIndex, int32(s.pool.TickSpacing), s.words, s.nets)
	c.retainedMu.Unlock()
	return missing
}

func (c *StateCache) captureRetainedWindow(ctx context.Context) (*StateCache, error) {
	c.retainedMu.Lock()
	if c.retained == nil {
		c.retainedMu.Unlock()
		return nil, fmt.Errorf("failed to capture turbos retained window: state=null")
	}
	floor := c.retainedHead.SequenceNumber
	if c.retained.position != nil && c.retained.position.Sequence > floor.Uint64() {
		floor = sui.CheckpointSequenceNumber(c.retained.position.Sequence)
	}
	c.retainedMu.Unlock()
	candidate, err := NewStateCache(c.reader, c.pool, c.maxAge)
	if err != nil {
		return nil, err
	}
	candidate.ObserveCheckpoint(floor)
	if err := candidate.Warm(ctx); err != nil {
		return nil, err
	}
	return candidate, nil
}

func (c *StateCache) installRetainedWindow(candidate *StateCache, updates []suiwindow.Update) error {
	if candidate == nil || candidate.retained == nil {
		return fmt.Errorf("failed to install turbos retained window: snapshot=null")
	}
	for _, update := range updates {
		if err := candidate.applyRetainedObjects(update.Notification, update.ReceivedAt); err != nil {
			return fmt.Errorf("failed to replay turbos retained window: %w", err)
		}
	}
	c.retainedMu.Lock()
	defer c.retainedMu.Unlock()
	old, next := c.retained, candidate.retained
	if old == nil || next == nil {
		return fmt.Errorf("failed to install turbos retained window: state=null")
	}
	if next.version < old.version || candidate.retainedHead.SequenceNumber < c.retainedHead.SequenceNumber {
		return fmt.Errorf("failed to install turbos retained window: position=regressed")
	}
	if next.version == old.version && next.digest != old.digest {
		return fmt.Errorf("failed to install turbos retained window: digest=mismatch")
	}
	c.retained, c.retainedHead = next, candidate.retainedHead
	c.publishQuoteSnapshotLocked()
	return nil
}
