package clmm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	sui "github.com/k4k3ru-hub/onchain/go/sui"
	"time"
)

// CheckState verifies pool identity and all previously consumed dynamic fields at one checkpoint.
// It performs no swap calculation and never uses trade progress as proof of state validity.
//
// Version:
//   - 2026-09-09: Added.
func (c *StateCache) CheckState(ctx context.Context) (result quotestate.Check, err error) {
	if c == nil || ctx == nil {
		return result, fmt.Errorf("failed to check turbos state: dependency=null")
	}
	select {
	case <-ctx.Done():
		return result, fmt.Errorf("failed to check quote state: %w", ctx.Err())
	case c.gate <- struct{}{}:
	}
	defer func() {
		if err != nil {
			c.state = nil
		}
		<-c.gate
	}()
	started := time.Now().UTC()
	head, err := c.reader.LatestCheckpoint(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	if head.Timestamp.IsZero() || time.Since(head.Timestamp) > c.maxAge || head.Timestamp.After(time.Now()) || head.SequenceNumber.Uint64() < c.floor.Load() {
		return result, fmt.Errorf("failed to check turbos state: checkpoint=invalid")
	}
	if head.SequenceNumber < c.accepted.SequenceNumber || head.Timestamp.Before(c.accepted.Timestamp) {
		return result, fmt.Errorf("failed to check turbos state: checkpoint=regressed")
	}
	if head.SequenceNumber == c.accepted.SequenceNumber && head.Digest != c.accepted.Digest {
		return result, fmt.Errorf("failed to check turbos state: checkpoint_digest=mismatch")
	}
	obj, err := c.reader.ObjectAtCheckpoint(ctx, c.pool, head.SequenceNumber)
	if err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	if obj == nil || obj.Address != c.pool || obj.Version == 0 {
		return result, fmt.Errorf("failed to check turbos state: object=invalid")
	}
	fresh, err := capturePool(obj, head.SequenceNumber)
	if err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}

	fields := map[string]json.RawMessage{}
	if old := c.state; old != nil {
		for index := range old.words {
			raw, e := c.field(ctx, fresh, fresh.bitmap, index)
			if e != nil {
				return result, fmt.Errorf("failed to check quote state: %w", e)
			}
			fields[fmt.Sprintf("word:%d", index)] = raw
		}
		for index := range old.nets {
			raw, e := c.field(ctx, fresh, fresh.ticks, index)
			if e != nil {
				return result, fmt.Errorf("failed to check quote state: %w", e)
			}
			fields[fmt.Sprintf("tick:%d", index)] = raw
		}
	}
	encoded, err := json.Marshal(struct {
		Version uint64
		Digest  sui.ObjectDigest
		Fields  map[string]json.RawMessage
	}{obj.Version, obj.Digest, fields})
	if err != nil {
		return result, fmt.Errorf("failed to encode turbos state: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	if head.SequenceNumber.Uint64() < c.floor.Load() {
		return result, fmt.Errorf("failed to check turbos state: snapshot=invalidated")
	}
	key := fmt.Sprintf("%x", sha256.Sum256(encoded))
	// Reuse decoded fields only after verifying every consumed field again.
	if c.checkedKey == key && c.state != nil {
		fresh.words = c.state.words
		fresh.nets = c.state.nets
	}
	c.checkedKey = key
	c.state = fresh
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
