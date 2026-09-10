package v4

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const retainedReplayLimit = 4096
const retainedWordRadius = 1

type retainedCaptureResult struct {
	snapshot *poolSnapshot
	err      error
}

// bitmapWord uses floor division, including negative ticks that are not spaced.
func bitmapWord(tick, spacing int32) int32 {
	compressed := tick / spacing
	if tick < 0 && tick%spacing != 0 {
		compressed--
	}
	return compressed >> 8
}

func (c *StateCache) retainedNeedsCapture(amount *big.Int, baseIsToken0 bool) bool {
	if c.retained == nil {
		return true
	}
	s := clonePoolSnapshot(c.retained)
	center := bitmapWord(s.tick, s.spacing)
	minWord, maxWord := bitmapWord(-887272, s.spacing), bitmapWord(887272, s.spacing)
	for word := max(center-retainedWordRadius, minWord); word <= min(center+retainedWordRadius, maxWord); word++ {
		if s.words[word] == nil {
			return true
		}
		for bit := 0; bit < 256; bit++ {
			if s.words[word].Bit(bit) != 0 && s.ticks[(word*256+int32(bit))*s.spacing] == nil {
				return true
			}
		}
	}
	// Also detect missing tick details after a liquidity change or price movement.
	budget := 0
	if _, err := c.quote(context.Background(), s, amount, baseIsToken0, true, &budget); err != nil {
		return errors.Is(err, errStateReadBudget)
	}
	_, err := c.quote(context.Background(), s, amount, !baseIsToken0, false, &budget)
	return errors.Is(err, errStateReadBudget)
}

func (c *StateCache) captureRetainedWindow(ctx context.Context, amount *big.Int, baseIsToken0 bool, floor uint64, hash common.Hash) (*poolSnapshot, error) {
	// A separate reader avoids mutating the legacy QuotePair cache or spacing.
	reader := &StateCache{rpc: c.rpc, pool: c.pool, stateView: c.stateView, poolID: c.poolID, fee: c.fee, spacing: c.spacing}
	budget := 64
	s, err := reader.capture(ctx, &budget, floor, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to capture retained window: %w", err)
	}
	center := bitmapWord(s.tick, s.spacing)
	minWord, maxWord := bitmapWord(-887272, s.spacing), bitmapWord(887272, s.spacing)
	lower, upper := max(center-retainedWordRadius, minWord), min(center+retainedWordRadius, maxWord)
	if err := reader.readBitmapWindow(ctx, s, lower, upper, &budget); err != nil {
		return nil, err
	}
	if err := reader.readWindowTicks(ctx, s, lower, upper); err != nil {
		return nil, err
	}
	// Reference quotes may fetch extra inputs outside the complete window.
	if _, err := reader.quote(ctx, s, amount, baseIsToken0, true, &budget); err != nil {
		return nil, fmt.Errorf("failed to capture retained bid coverage: %w", err)
	}
	if _, err := reader.quote(ctx, s, amount, !baseIsToken0, false, &budget); err != nil {
		return nil, fmt.Errorf("failed to capture retained ask coverage: %w", err)
	}
	header, err := reader.rpc.HeaderByNumber(ctx, s.header.Number)
	if err != nil {
		return nil, fmt.Errorf("failed to verify retained window: %w", err)
	}
	if header.Number != s.header.Number || header.Hash != s.header.Hash {
		return nil, fmt.Errorf("failed to verify retained window: block_hash=mismatch")
	}
	return s, nil
}

func (c *StateCache) runRetainedSubscription(ctx context.Context, ws WSRPCClient, amount *big.Int, baseIsToken0 bool) error {
	if ctx == nil || ws == nil {
		return fmt.Errorf("failed to run retained state: dependency=null")
	}
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("failed to run retained state: subscription=active")
	}
	c.running = true
	c.generation++
	c.floorHash = common.Hash{}
	c.retained = nil
	c.retainedBaseAmount = amount
	recovery := make(chan struct{}, 1)
	c.retainedRecovery = recovery
	c.publishQuoteSnapshotLocked()
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.running, c.active = false, false
		c.generation++
		c.retained = nil
		c.publishQuoteSnapshotLocked()
		c.mu.Unlock()
	}()
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	logs := make(chan types.Log, retainedReplayLimit)
	sub, err := ws.SubscribeFilterLogs(ctx, ethereum.FilterQuery{Addresses: []common.Address{c.pool}, Topics: [][]common.Hash{nil, {c.poolID}}}, logs)
	if err != nil {
		return fmt.Errorf("failed to subscribe retained logs: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("failed to subscribe retained logs: subscription=null")
	}
	defer sub.Unsubscribe()
	c.mu.Lock()
	c.active = true
	c.mu.Unlock()
	results := make(chan retainedCaptureResult, 1)
	type receivedLog struct {
		log types.Log
		at  time.Time
	}
	var replay []receivedLog
	busy, pending := false, true
	delay := time.Second
	var nextAttempt time.Time
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		if pending && !busy && !time.Now().Before(nextAttempt) {
			c.mu.Lock()
			var floor uint64
			var hash common.Hash
			if c.retained != nil {
				floor, hash = c.retainedBlock, c.retainedHash
			}
			c.mu.Unlock()
			busy, pending = true, false
			replay = nil
			workers.Add(1)
			go func() {
				defer workers.Done()
				readCtx, stop := context.WithTimeout(ctx, 30*time.Second)
				defer stop()
				s, err := c.captureRetainedWindow(readCtx, amount, baseIsToken0, floor, hash)
				select {
				case results <- retainedCaptureResult{s, err}:
				case <-ctx.Done():
				}
			}()
		}
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		case <-recovery:
			pending = true
		case err, ok := <-sub.Err():
			if !ok || err == nil {
				return fmt.Errorf("failed to watch retained state: subscription=closed")
			}
			return fmt.Errorf("failed to watch retained state: %w", err)
		case result := <-results:
			busy = false
			if result.err != nil {
				// Keep usable streamed inputs during a failed refresh. Initial
				// capture errors remain visible to the application's retry loop.
				c.mu.Lock()
				hasState := c.retained != nil
				c.mu.Unlock()
				if !hasState {
					return result.err
				}
				pending = true
				nextAttempt = time.Now().Add(delay)
				timer.Reset(delay)
				delay = min(delay*2, 30*time.Second)
				continue
			}
			candidate := &StateCache{fee: c.fee, retained: result.snapshot, retainedBlock: result.snapshot.header.Number, retainedHash: result.snapshot.header.Hash}
			for _, log := range replay {
				if err := candidate.applyRetainedLog(log.log, log.at); err != nil {
					return fmt.Errorf("failed to replay retained window: %w", err)
				}
			}
			replay = nil
			c.mu.Lock()
			c.retained = candidate.retained
			c.retainedBlock, c.retainedHash = candidate.retainedBlock, candidate.retainedHash
			c.retainedIndex, c.retainedHasLog = candidate.retainedIndex, candidate.retainedHasLog
			pending = c.retainedNeedsCapture(amount, baseIsToken0)
			c.publishQuoteSnapshotLocked()
			c.mu.Unlock()
			delay = time.Second
			nextAttempt = time.Now().Add(delay)
			timer.Reset(delay)
		case log, ok := <-logs:
			receivedAt := time.Now().UTC()
			if !ok {
				return fmt.Errorf("failed to watch retained state: logs=closed")
			}
			if log.Address != c.pool || len(log.Topics) < 2 || log.Topics[1] != c.poolID {
				continue
			}
			if log.Removed || log.BlockHash == (common.Hash{}) {
				return fmt.Errorf("failed to apply retained pool log: state requires reinitialization")
			}
			if busy {
				if len(replay) >= retainedReplayLimit {
					return fmt.Errorf("failed to buffer retained logs: replay=too_long max_length=%d", retainedReplayLimit)
				}
				replay = append(replay, receivedLog{log, receivedAt})
			}
			c.mu.Lock()
			if c.retained != nil {
				err := c.applyRetainedLog(log, receivedAt)
				if err != nil {
					c.mu.Unlock()
					return err
				}
				pending = pending || c.retainedNeedsCapture(amount, baseIsToken0)
				c.publishQuoteSnapshotLocked()
			}
			c.mu.Unlock()
		}
	}
}
