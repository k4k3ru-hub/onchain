package clmm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/internal/suiwindow"
	"github.com/k4k3ru-hub/onchain/go/sui"
)

// Cetus has keyed skip-list nodes, not bitmap words. Three aligned intervals
// cover at most 768 candidate tick positions, irrespective of total pool size.
type tickWindow struct{ center, spacing, first, last, lower, upper int32 }

func newTickWindow(tick, spacing int32) (*tickWindow, error) {
	if spacing <= 0 || spacing > 443636 || tick < -443636 || tick > 443636 {
		return nil, fmt.Errorf("failed to define cetus tick window: parameters=out_of_range")
	}
	center := suiwindow.Word(tick, spacing)
	first := max(int64(center-1)*256, int64(-443636)/int64(spacing))
	last := min(int64(center+2)*256-1, int64(443636)/int64(spacing))
	return &tickWindow{center: center, spacing: spacing, first: int32(first * int64(spacing)), last: int32(last * int64(spacing)), lower: int32(max(int64(-443636), int64(center-1)*256*int64(spacing))), upper: int32(min(int64(443636), int64(center+2)*256*int64(spacing)))}, nil
}

func retainedTickSpacing(obj *sui.Object) (int32, error) {
	if obj == nil || obj.Move == nil {
		return 0, fmt.Errorf("failed to read cetus tick spacing: object=null")
	}
	var fields struct {
		Spacing json.RawMessage `json:"tick_spacing"`
	}
	if err := json.Unmarshal(obj.Move.JSON, &fields); err != nil {
		return 0, fmt.Errorf("failed to read cetus tick spacing: %w", err)
	}
	n, err := jsonUint64(fields.Spacing)
	if err != nil {
		return 0, fmt.Errorf("failed to read cetus tick spacing: %w", err)
	}
	if n == 0 || n > 443636 {
		return 0, fmt.Errorf("failed to read cetus tick spacing: spacing=out_of_range")
	}
	return int32(n), nil
}

func (c *StateCache) retainedCoverageMissing() bool {
	c.retainedMu.Lock()
	defer c.retainedMu.Unlock()
	if c.retained == nil || c.retained.snapshot.window == nil {
		return false
	}
	s := c.retained.snapshot
	return suiwindow.Word(s.Pool.CurrentTickIndex, s.window.spacing) != s.window.center
}

func (c *StateCache) captureRetainedWindow(ctx context.Context) (*StateCache, error) {
	c.retainedMu.Lock()
	if c.retained == nil {
		c.retainedMu.Unlock()
		return nil, fmt.Errorf("failed to capture cetus retained window: state=null")
	}
	floor := max(c.retained.baseline.SequenceNumber, sui.CheckpointSequenceNumber(c.floor.Load()))
	if p := c.retained.position; p != nil && p.Sequence > floor.Uint64() {
		floor = sui.CheckpointSequenceNumber(p.Sequence)
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
		return fmt.Errorf("failed to install cetus retained window: snapshot=null")
	}
	for _, update := range updates {
		if err := candidate.applyRetainedObjects(update.Notification, update.ReceivedAt); err != nil {
			return fmt.Errorf("failed to replay cetus retained window: %w", err)
		}
	}
	c.retainedMu.Lock()
	defer c.retainedMu.Unlock()
	old, next := c.retained, candidate.retained
	if old == nil || next == nil {
		return fmt.Errorf("failed to install cetus retained window: state=null")
	}
	if next.version < old.version || next.baseline.SequenceNumber < old.baseline.SequenceNumber {
		return fmt.Errorf("failed to install cetus retained window: position=regressed")
	}
	if next.version == old.version && next.digest != old.digest {
		return fmt.Errorf("failed to install cetus retained window: digest=mismatch")
	}
	oldSequence, nextSequence := old.baseline.SequenceNumber.Uint64(), next.baseline.SequenceNumber.Uint64()
	if old.position != nil {
		oldSequence = old.position.Sequence
	}
	if next.position != nil {
		nextSequence = next.position.Sequence
	}
	if nextSequence < oldSequence {
		return fmt.Errorf("failed to install cetus retained window: checkpoint=regressed")
	}
	if old.received.After(next.received) {
		next.received = old.received
	}
	c.retained = next
	c.publishQuoteSnapshotLocked()
	return nil
}
