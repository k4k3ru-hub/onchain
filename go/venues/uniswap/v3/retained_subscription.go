package v3

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
const retainedWordRadius = 2

type retainedCaptureResult struct {
	snapshot *poolSnapshot
	err      error
	epoch    uint64
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
	reader := &StateCache{rpc: c.rpc, pool: c.pool, fee: c.fee}
	budget := 64
	s, err := reader.capture(ctx, &budget, floor, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to capture retained window: %w", err)
	}
	center := bitmapWord(s.tick, s.spacing)
	minWord, maxWord := bitmapWord(-887272, s.spacing), bitmapWord(887272, s.spacing)
	if err := reader.readBitmapWindow(ctx, s, max(center-retainedWordRadius, minWord), min(center+retainedWordRadius, maxWord), &budget); err != nil {
		return nil, err
	}
	// Only fetch tick details required by the reference pair. A sparse pool may
	// require additional words; all contract reads share the existing 64-call cap.
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

func (c *StateCache) runRetainedSubscription(ctx context.Context, ws WSRPCClient, amount *big.Int, baseIsToken0 bool, events *RetainedEvents) error {
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
	sub, err := ws.SubscribeFilterLogs(ctx, ethereum.FilterQuery{Addresses: []common.Address{c.pool}}, logs)
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
	if events != nil && events.OnSubscribed != nil {
		events.OnSubscribed()
	}
	results := make(chan retainedCaptureResult, 1)
	type receivedLog struct {
		log types.Log
		at  time.Time
	}
	var replay []receivedLog
	busy, pending := false, true
	delay := time.Second
	var nextAttempt time.Time
	var epoch uint64
	var observedBlock, recoveryFloor uint64
	var captureCancel context.CancelFunc
	var order retainedLogOrder
	invalidate := func(err error) {
		epoch++
		recoveryFloor = observedBlock
		if captureCancel != nil {
			captureCancel()
		}
		replay = nil
		c.mu.Lock()
		c.retained = nil
		c.generation++
		c.publishQuoteSnapshotLocked()
		c.mu.Unlock()
		pending = true
		nextAttempt = time.Now().Add(delay)
		if events.OnRecovery != nil {
			events.OnRecovery(err)
		}
	}

	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		if pending && !busy && !time.Now().Before(nextAttempt) {
			c.mu.Lock()
			floor := recoveryFloor
			var hash common.Hash
			if c.retained != nil {
				floor, hash = c.retainedBlock, c.retainedHash
				replay = nil
			}
			c.mu.Unlock()
			busy, pending = true, false
			readCtx, stop := context.WithTimeout(ctx, 30*time.Second)
			captureCancel = stop
			captureEpoch := epoch
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer stop()
				s, err := c.captureRetainedWindow(readCtx, amount, baseIsToken0, floor, hash)
				select {
				case results <- retainedCaptureResult{snapshot: s, err: err, epoch: captureEpoch}:
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
			captureCancel = nil
			if result.epoch != epoch {
				timer.Reset(max(time.Until(nextAttempt), time.Millisecond))
				continue
			}
			if result.err != nil {
				// Keep usable streamed inputs during a failed refresh. Initial
				// capture errors remain visible to the application's retry loop.
				c.mu.Lock()
				hasState := c.retained != nil
				c.mu.Unlock()
				if !hasState && events == nil {
					return result.err
				}
				if events != nil && events.OnRecovery != nil {
					events.OnRecovery(result.err)
				}
				pending = true
				nextAttempt = time.Now().Add(delay)
				timer.Reset(delay)
				delay = min(delay*2, 30*time.Second)
				continue
			}
			candidate := &StateCache{retained: result.snapshot, retainedBlock: result.snapshot.header.Number, retainedHash: result.snapshot.header.Hash}
			var replayErr error
			for _, log := range replay {
				if err := candidate.applyRetainedLog(log.log, log.at); err != nil {
					replayErr = fmt.Errorf("failed to replay retained window: %w", err)
					break
				}
			}
			if replayErr != nil {
				if events == nil {
					return replayErr
				}
				invalidate(replayErr)
				timer.Reset(delay)
				continue
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
			if log.Address != c.pool {
				continue
			}
			if events != nil {
				observedBlock = max(observedBlock, log.BlockNumber)
				duplicate, err := order.accept(log)
				if duplicate {
					continue
				}
				if err != nil {
					invalidate(err)
					timer.Reset(delay)
					continue
				}
			} else {
				if log.Removed {
					return retainedReinitializationError(log, "removed log")
				}
				if log.BlockHash == (common.Hash{}) {
					return retainedReinitializationError(log, "missing block hash")
				}
			}
			c.mu.Lock()
			missingState := c.retained == nil
			c.mu.Unlock()
			if busy || (events != nil && missingState) {
				if len(replay) >= retainedReplayLimit {
					err := fmt.Errorf("failed to buffer retained logs: replay=too_long max_length=%d", retainedReplayLimit)
					if events == nil {
						return err
					}
					invalidate(err)
					timer.Reset(delay)
				}
				replay = append(replay, receivedLog{log, receivedAt})
			}
			c.mu.Lock()
			if c.retained != nil {
				err := c.applyRetainedLog(log, receivedAt)
				if err != nil {
					c.mu.Unlock()
					if events == nil {
						return err
					}
					invalidate(err)
					timer.Reset(delay)
					continue
				}
				pending = pending || c.retainedNeedsCapture(amount, baseIsToken0)
				c.publishQuoteSnapshotLocked()
			}
			c.mu.Unlock()
			if events != nil && events.OnSwap != nil && len(log.Topics) > 0 && log.Topics[0] == swapEventSignatureHash() {
				swap, err := DecodeSwapLog(log)
				if err != nil {
					invalidate(fmt.Errorf("failed to decode retained live swap: %w", err))
					timer.Reset(delay)
					continue
				}
				if err := events.OnSwap(ctx, swap, receivedAt); err != nil {
					return fmt.Errorf("failed to consume retained live swap: %w", err)
				}
			}
		}
	}
}
