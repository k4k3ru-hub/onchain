package clmm

import (
	"context"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"time"
)

// Warm captures pool state and the complete bounded tick window without a trial quote.
//
// Version:
//   - 2026-09-12: Added.
func (c *StateCache) Warm(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("failed to warm momentum state: cache=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("failed to warm momentum state: %w", ctx.Err())
	}
	defer func() { <-c.gate }()
	head, err := sui.QuoteCheckpoint(ctx, c.reader, sui.CheckpointSequenceNumber(c.floor.Load()))
	if err != nil {
		return fmt.Errorf("failed to warm momentum state: %w", err)
	}
	if head.Timestamp.IsZero() || time.Since(head.Timestamp) > c.maxAge {
		return fmt.Errorf("failed to warm momentum state: checkpoint=invalid")
	}
	if head.SequenceNumber.Uint64() < c.floor.Load() {
		return fmt.Errorf("failed to verify quote checkpoint: %w: checkpoint=behind", quotestate.ErrStateChanged)
	}
	if head.SequenceNumber < c.accepted.SequenceNumber || head.Timestamp.Before(c.accepted.Timestamp) {
		return fmt.Errorf("failed to warm momentum state: checkpoint=regressed")
	}
	if head.SequenceNumber == c.accepted.SequenceNumber && head.Digest != c.accepted.Digest {
		return fmt.Errorf("failed to warm momentum state: checkpoint_digest=mismatch")
	}

	obj, err := c.reader.ObjectAtCheckpoint(ctx, c.pool, head.SequenceNumber)
	receivedAt := time.Now().UTC()
	if err != nil {
		return fmt.Errorf("failed to warm momentum state: %w", err)
	}
	if obj == nil || obj.Address != c.pool || obj.Version == 0 {
		return fmt.Errorf("failed to warm momentum state: object=invalid")
	}
	if c.state == nil || obj.Version != c.state.version || obj.Digest != c.state.digest || time.Since(c.state.captured) > c.maxAge {
		state, err := capturePool(obj, head.SequenceNumber)
		if err != nil {
			return fmt.Errorf("failed to warm momentum state: %w", err)
		}
		if err := c.checkTrading(ctx, state); err != nil {
			return err
		}
		if err := c.captureWindow(ctx, state); err != nil {
			return err
		}
		c.state = state
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("failed to warm momentum state: %w", err)
	}
	if head.SequenceNumber.Uint64() < c.floor.Load() || time.Since(head.Timestamp) > c.maxAge || time.Since(c.state.captured) > c.maxAge {
		return fmt.Errorf("failed to warm momentum state: %w: snapshot=invalidated", quotestate.ErrStateChanged)
	}
	c.accepted = head
	c.retainedMu.Lock()
	c.state.received = receivedAt
	c.retained = cloneRetainedState(c.state)
	c.retained.retainedOnly = true
	c.retainedHead = head
	c.retainedReference = nil
	c.publishQuoteSnapshotLocked()
	c.retainedMu.Unlock()
	return nil
}
