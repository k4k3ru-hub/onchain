package clmm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"sort"
	"time"
)

// CheckState verifies the pool and previously consumed tick state without calculating a quote.
//
// Version:
//   - 2026-09-09: Classify state-change retries separately from transport failures.
//   - 2026-09-09: Added.
func (c *StateCache) CheckState(ctx context.Context) (result quotestate.Check, err error) {
	if c == nil || ctx == nil {
		return result, fmt.Errorf("failed to check cetus state: dependency=null")
	}
	select {
	case <-ctx.Done():
		return result, fmt.Errorf("failed to check quote state: %w", ctx.Err())
	case c.gate <- struct{}{}:
	}
	defer func() {
		if err != nil {
			c.snapshot = nil
		}
		<-c.gate
	}()
	started := time.Now().UTC()
	head, err := sui.QuoteCheckpoint(ctx, c.reader, sui.CheckpointSequenceNumber(c.floor.Load()))
	if err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	if head.Timestamp.IsZero() || time.Since(head.Timestamp) > c.maxAge || head.Timestamp.After(time.Now()) {
		return result, fmt.Errorf("failed to check cetus state: checkpoint=invalid")
	}
	if head.SequenceNumber.Uint64() < c.floor.Load() {
		return result, fmt.Errorf("failed to verify quote checkpoint: %w: checkpoint=behind", quotestate.ErrStateChanged)
	}
	if head.SequenceNumber < c.accepted.SequenceNumber || head.Timestamp.Before(c.accepted.Timestamp) {
		return result, fmt.Errorf("failed to check cetus state: checkpoint=regressed")
	}
	if head.SequenceNumber == c.accepted.SequenceNumber && head.Digest != c.accepted.Digest {
		return result, fmt.Errorf("failed to check cetus state: checkpoint_digest=mismatch")
	}
	obj, err := c.reader.ObjectAtCheckpoint(ctx, c.pool, head.SequenceNumber)
	if err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	if obj == nil || obj.Address != c.pool || obj.Version == 0 {
		return result, fmt.Errorf("failed to check cetus state: object=invalid")
	}
	// Verify the exact tick read set and linked-list edges used by the cached quote.
	// Children can change independently, so the parent object version alone is insufficient.
	var fields []json.RawMessage
	if c.snapshot != nil {
		if r, ok := c.reader.(neighborReader); ok {
			var meta struct {
				Manager struct {
					Ticks struct {
						ID string `json:"id"`
					} `json:"ticks"`
				} `json:"tick_manager"`
			}
			if obj.Move == nil {
				return result, fmt.Errorf("failed to check cetus state: move=null")
			}
			if err := json.Unmarshal(obj.Move.JSON, &meta); err != nil {
				return result, fmt.Errorf("failed to check cetus state: %w", err)
			}
			handle, e := sui.ParseAddress(meta.Manager.Ticks.ID)
			if e != nil {
				return result, fmt.Errorf("failed to check cetus state: %w", e)
			}
			keys := make([]uint64, 0, len(c.snapshot.Ticks))
			for _, tick := range c.snapshot.Ticks {
				keys = append(keys, uint64(int64(tick.Index)+443636))
			}
			sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
			for from := 0; from < len(keys); from += 50 {
				to := min(from+50, len(keys))
				values, e := r.DynamicUint64ValuesAtCheckpoint(ctx, handle, head.SequenceNumber, keys[from:to])
				if e != nil {
					return result, fmt.Errorf("failed to check cetus ticks: %w", e)
				}
				if len(values) != to-from {
					return result, fmt.Errorf("failed to check cetus ticks: count=mismatch")
				}
				fields = append(fields, values...)
			}
		} else {
			snapshot, e := captureState(ctx, c.reader, obj, head.SequenceNumber)
			if e != nil {
				return result, fmt.Errorf("failed to check cetus ticks: %w", e)
			}
			raw, e := json.Marshal(snapshot.Ticks)
			if e != nil {
				return result, fmt.Errorf("failed to encode cetus ticks: %w", e)
			}
			fields = append(fields, raw)
		}
	}
	encoded, err := json.Marshal(struct {
		Version uint64
		Digest  sui.ObjectDigest
		Fields  []json.RawMessage
	}{obj.Version, obj.Digest, fields})
	if err != nil {
		return result, fmt.Errorf("failed to encode cetus state: %w", err)
	}
	key := fmt.Sprintf("%x", sha256.Sum256(encoded))
	if err = ctx.Err(); err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	if head.SequenceNumber.Uint64() < c.floor.Load() {
		return result, fmt.Errorf("failed to check cetus state: %w: snapshot=invalidated", quotestate.ErrStateChanged)
	}
	if c.checkedKey != key {
		c.snapshot = nil
	}
	c.checkedKey = key
	c.version = obj.Version
	c.digest = obj.Digest
	c.observed = started
	c.accepted = head
	return quotestate.Check{Key: key, Position: head.SequenceNumber.Uint64(), CheckedAt: started}, nil
}

// CheckCurrent reports whether verified state covers all observed pool changes.
//
// Version:
//   - 2026-09-09: Added.
func (c *StateCache) CheckCurrent(check quotestate.Check) bool {
	return c != nil && check.Key != "" && check.Position != 0 && c.floor.Load() <= check.Position
}
