package slipstream

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

const retainedWordRadius = 1

func bitmapWord(tick, spacing int32) int32 {
	compressed := tick / spacing
	if tick < 0 && tick%spacing != 0 {
		compressed--
	}
	return compressed >> 8
}

// retainedNeedsCapture runs under mu and never performs RPC.
func (c *StateCache) retainedNeedsCapture(amount *big.Int, baseIsToken0 bool) bool {
	if c.retained == nil || c.retained.pool == nil {
		return true
	}
	s := cloneRetainedPool(c.retained).pool
	center := bitmapWord(s.tick, s.spacing)
	for word := max(center-retainedWordRadius, bitmapWord(-887272, s.spacing)); word <= min(center+retainedWordRadius, bitmapWord(887272, s.spacing)); word++ {
		bitmap := s.words[word]
		if bitmap == nil {
			return true
		}
		for bit := 0; bit < 256; bit++ {
			if bitmap.Bit(bit) != 0 && s.ticks[(word*256+int32(bit))*s.spacing] == nil {
				return true
			}
		}
	}
	budget := 0
	if _, err := c.quote(context.Background(), s, amount, baseIsToken0, true, &budget); err != nil {
		return errors.Is(err, errStateReadBudget)
	}
	_, err := c.quote(context.Background(), s, amount, !baseIsToken0, false, &budget)
	return errors.Is(err, errStateReadBudget)
}

type retainedWindowLog struct {
	log       types.Log
	timestamp uint64
	received  time.Time
}

// installRetainedWindow replays all concurrent inputs before publishing a candidate.
// It runs under mu and restores the live state on any candidate failure.
func (c *StateCache) installRetainedWindow(next *retainedPoolState, logs []retainedWindowLog) error {
	previous, base, replayState, replayLogs := c.retained, c.replayBase, c.replayState, c.replayLogs
	if next == nil || next.pool == nil || previous == nil || next.fee.module != previous.fee.module {
		return fmt.Errorf("failed to install retained window: state=invalid")
	}
	floor := previous.pool.header.Number
	if previous.hasLog {
		floor = max(floor, previous.block)
	}
	c.retained, c.replayBase, c.replayState, c.replayLogs = next, nil, nil, nil
	rollback := func(err error) error {
		c.retained, c.replayBase, c.replayState, c.replayLogs = previous, base, replayState, replayLogs
		return err
	}
	for _, input := range logs {
		if err := c.applyRetainedLog(input.log, input.timestamp, input.received); err != nil {
			return rollback(fmt.Errorf("failed to replay retained window: %w", err))
		}
	}
	position := c.retained.pool.header.Number
	if c.retained.hasLog {
		position = max(position, c.retained.block)
	}
	if position < floor {
		return rollback(fmt.Errorf("failed to install retained window: block=regressed"))
	}
	c.publishQuoteSnapshotLocked()
	return nil
}
