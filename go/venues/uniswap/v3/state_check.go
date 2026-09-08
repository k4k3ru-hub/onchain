package v3

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"math/big"
	"time"
)

// Changes returns coalesced state and subscription lifecycle notifications.
// The channel has one consumer and is never closed.
//
// Version:
//   - 2026-09-09: Added.
func (c *StateCache) Changes() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.updates == nil {
		c.updates = make(chan struct{}, 1)
	}
	return c.updates
}
func (c *StateCache) signalChange() {
	select {
	case c.updates <- struct{}{}:
	default:
	}
}

// CheckState captures and verifies core state and every previously used tick word.
// It performs no swap calculation. Equal keys permit reuse of an existing quote.
// Original quote timestamps must not be replaced by CheckedAt.
//
// Version:
//   - 2026-09-09: Classify state-change retries separately from transport failures.
//   - 2026-09-09: Added.
func (c *StateCache) CheckState(ctx context.Context) (result quotestate.Check, err error) {
	if c == nil || ctx == nil {
		return result, fmt.Errorf("failed to check uniswap v3 state: dependency=null")
	}
	select {
	case <-ctx.Done():
		return result, fmt.Errorf("failed to check quote state: %w", ctx.Err())
	case <-c.gate:
	}
	defer func() {
		if err != nil {
			c.snapshot = nil
		}
		c.gate <- struct{}{}
	}()
	c.mu.Lock()
	gen, floor, hash := c.generation, c.floor, c.floorHash
	c.mu.Unlock()
	started := time.Now().UTC()
	budget := 256
	fresh, err := c.capture(ctx, &budget, floor, hash)
	if err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	old := c.snapshot
	if old != nil {
		if fresh.header.Number < old.header.Number {
			return result, fmt.Errorf("failed to check uniswap v3 state: block=regressed")
		}
		for index := range old.words {
			data, readErr := c.read(ctx, fresh, "tickBitmap(int16)", big.NewInt(int64(index)), &budget)
			if readErr != nil {
				return result, fmt.Errorf("failed to read quote state: %w", readErr)
			}
			if len(data) != 32 {
				return result, fmt.Errorf("failed to check uniswap v3 state: bitmap=invalid")
			}
			fresh.words[index] = new(big.Int).SetBytes(data)
		}
		for index := range old.ticks {
			compressed := index / fresh.spacing
			if index < 0 && index%fresh.spacing != 0 {
				compressed--
			}
			if word := fresh.words[compressed>>8]; word == nil || word.Bit(int(compressed&255)) == 0 {
				continue
			}
			if _, readErr := c.liquidityNet(ctx, fresh, index, &budget); readErr != nil {
				return result, fmt.Errorf("failed to read quote state: %w", readErr)
			}
		}
	}
	header, err := c.rpc.HeaderByNumber(ctx, fresh.header.Number)
	if err != nil {
		return result, fmt.Errorf("failed to verify uniswap v3 state: %w", err)
	}
	if header.Hash != fresh.header.Hash || header.Number != fresh.header.Number {
		return result, fmt.Errorf("failed to check uniswap v3 state: block_hash=mismatch")
	}
	if err = ctx.Err(); err != nil {
		return result, fmt.Errorf("failed to check quote state: %w", err)
	}
	reorg := false
	if old != nil {
		canonical := header
		if old.header.Number != header.Number {
			canonical, err = c.rpc.HeaderByNumber(ctx, old.header.Number)
			if err != nil {
				return result, fmt.Errorf("failed to verify previous quote block: %w", err)
			}
			if canonical.Number != old.header.Number {
				return result, fmt.Errorf("failed to verify previous quote block: number=mismatch")
			}
		}
		reorg = canonical.Hash != old.header.Hash
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.generation {
		return result, fmt.Errorf("failed to check quote state: %w: snapshot=invalidated", quotestate.ErrStateChanged)
	}
	if reorg {
		c.verificationEpoch++
	}
	encoded, err := json.Marshal(struct {
		Epoch            uint64
		Price, Liquidity *big.Int
		Tick, Spacing    int32
		Fee              uint32
		Fees             [2]uint32
		Words, Ticks     map[int32]*big.Int
	}{Epoch: c.verificationEpoch, Price: fresh.price, Liquidity: fresh.liquidity, Tick: fresh.tick, Spacing: fresh.spacing, Words: fresh.words, Ticks: fresh.ticks})
	if err != nil {
		return result, fmt.Errorf("failed to encode uniswap v3 state: %w", err)
	}
	c.snapshot = fresh
	c.covered = fresh.header
	c.cachedGeneration = gen
	return quotestate.Check{Revision: gen, Key: fmt.Sprintf("%x", sha256.Sum256(encoded)), Position: fresh.header.Number, CheckedAt: started}, nil
}

// CheckCurrent reports whether notifications received since verification invalidate the check.
//
// Version:
//   - 2026-09-09: Added.
func (c *StateCache) CheckCurrent(check quotestate.Check) bool {
	if c == nil || check.Key == "" || check.Position == 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation == check.Revision
}
