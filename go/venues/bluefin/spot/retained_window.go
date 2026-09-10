package spot

import (
	"context"
	"errors"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/internal/suiwindow"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"math/big"
)

func (c *StateCache) captureWindow(ctx context.Context, s *quoteState) error {
	words, nets, err := suiwindow.Read(ctx, c.reader, s.checkpoint, s.bitmap, s.ticks, s.keyType, s.tickIndex, int32(s.spacing), jsonUnsigned)
	if err != nil {
		return fmt.Errorf("failed to capture bluefin tick window: %w", err)
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

	return p
}

func (c *StateCache) retainedCoverageMissing(ctx context.Context) bool {
	c.retainedMu.Lock()
	s := c.retained
	reference := c.retainedReference
	if s == nil {
		c.retainedMu.Unlock()
		return false
	}
	missing := suiwindow.Missing(s.tickIndex, int32(s.spacing), s.words, s.nets)
	c.retainedMu.Unlock()
	if missing {
		return true
	}
	if reference == nil {
		return false
	}
	_, err := c.QuoteRetainedPair(ctx, *reference)
	return errors.Is(err, suiwindow.ErrCoverage)
}

func (c *StateCache) captureRetainedWindow(ctx context.Context) (*StateCache, error) {
	c.retainedMu.Lock()
	if c.retained == nil || c.retainedReference == nil {
		c.retainedMu.Unlock()
		return nil, fmt.Errorf("failed to capture bluefin retained window: state=null")
	}
	reference := cloneReference(*c.retainedReference)
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
	if _, err := candidate.QuotePair(ctx, reference); err != nil {
		return nil, err
	}
	return candidate, nil
}

func (c *StateCache) installRetainedWindow(candidate *StateCache, updates []suiwindow.Update) error {
	if candidate == nil || candidate.retained == nil {
		return fmt.Errorf("failed to install bluefin retained window: snapshot=null")
	}
	for _, update := range updates {
		if err := candidate.applyRetainedObjects(update.Notification, update.ReceivedAt); err != nil {
			return fmt.Errorf("failed to replay bluefin retained window: %w", err)
		}
	}
	c.retainedMu.Lock()
	defer c.retainedMu.Unlock()
	old, next := c.retained, candidate.retained
	if old == nil || next == nil {
		return fmt.Errorf("failed to install bluefin retained window: state=null")
	}
	if next.version < old.version || candidate.retainedHead.SequenceNumber < c.retainedHead.SequenceNumber {
		return fmt.Errorf("failed to install bluefin retained window: position=regressed")
	}
	if next.version == old.version && next.digest != old.digest {
		return fmt.Errorf("failed to install bluefin retained window: digest=mismatch")
	}
	c.retained, c.retainedHead = next, candidate.retainedHead
	c.publishQuoteSnapshotLocked()
	return nil
}
