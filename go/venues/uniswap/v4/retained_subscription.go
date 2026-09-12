package v4

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const retainedReplayLimit = 4096
const retainedWordRadius = 1

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
	if s.windowCaptured && center != s.windowCenter {
		return true
	}
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
	return false
}

// retainedWindowCapture is private to one uninterrupted subscription epoch.
// Keep its baseline and completed batches across attempt deadlines and backoff.
type retainedWindowCapture struct {
	snapshot    *poolSnapshot
	bitmapReady bool
	ticks       retainedTickProgress
}

func (c *StateCache) captureRetainedWindow(ctx context.Context, amount *big.Int, baseIsToken0 bool, floor uint64, hash common.Hash) (*poolSnapshot, error) {
	return c.resumeRetainedWindow(ctx, floor, hash, &retainedWindowCapture{})
}

func (c *StateCache) resumeRetainedWindow(ctx context.Context, floor uint64, hash common.Hash, progress *retainedWindowCapture) (*poolSnapshot, error) {
	// A separate reader avoids mutating the legacy QuotePair cache or spacing.
	reader := &StateCache{rpc: c.rpc, pool: c.pool, stateView: c.stateView, poolID: c.poolID, fee: c.fee, spacing: c.spacing}
	budget := 64
	if progress.snapshot == nil {
		progress.ticks.started = time.Now()
		s, err := reader.capture(ctx, &budget, floor, hash)
		if err != nil {
			return nil, fmt.Errorf("failed to capture retained window: %w", err)
		}
		progress.snapshot = s
	} else if err := reader.verifyRetainedCapture(ctx, progress); err != nil {
		return nil, err
	}
	s := progress.snapshot
	center := bitmapWord(s.tick, s.spacing)
	s.windowCenter, s.windowCaptured = center, true
	lower := max(center-retainedWordRadius, bitmapWord(-887272, s.spacing))
	upper := min(center+retainedWordRadius, bitmapWord(887272, s.spacing))
	if !progress.bitmapReady {
		if err := reader.readBitmapWindow(ctx, s, lower, upper, &budget); err != nil {
			return nil, err
		}
		progress.bitmapReady = true
	}
	if err := reader.resumeWindowTicks(ctx, s, lower, upper, &progress.ticks); err != nil {
		return nil, err
	}
	if err := reader.verifyRetainedCapture(ctx, progress); err != nil {
		return nil, err
	}
	return s, nil
}

func (c *StateCache) verifyRetainedCapture(ctx context.Context, progress *retainedWindowCapture) error {
	s := progress.snapshot
	header, err := c.rpc.HeaderByNumber(ctx, s.header.Number)
	if err != nil {
		return fmt.Errorf("failed to verify retained window: %w: block_number=%d", err, s.header.Number)
	}
	if header.Number != s.header.Number || header.Hash != s.header.Hash {
		// The same number no longer identifies the data we staged. Restart all
		// fields together; never combine ticks from different block hashes.
		*progress = retainedWindowCapture{}
		return fmt.Errorf("failed to verify retained window: block_hash=mismatch block_number=%d", s.header.Number)
	}
	return nil
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
	if events == nil {
		c.retained = nil
	}
	c.retainedBaseAmount = amount
	recovery := make(chan struct{}, 1)
	c.retainedRecovery = recovery
	c.publishQuoteSnapshotLocked()
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.running, c.active = false, false
		c.generation++
		if events == nil {
			c.retained = nil
		}
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
	if events != nil && events.OnSubscribed != nil {
		events.OnSubscribed()
	}
	results := make(chan retainedCaptureResult, 1)
	type receivedLog struct {
		log types.Log
		at  time.Time
	}
	var replay []receivedLog
	c.mu.Lock()
	needsCapture := c.retainedNeedsCapture(amount, baseIsToken0)
	c.mu.Unlock()
	busy, pending := false, needsCapture
	delay := time.Second
	var nextAttempt time.Time
	var epoch uint64
	var observedBlock, recoveryFloor uint64
	var captureCancel context.CancelFunc
	var capture *retainedWindowCapture
	order := &c.liveOrder
	invalidate := func(err error) {
		epoch++
		recoveryFloor = observedBlock
		capture = nil
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
			if capture == nil {
				capture = &retainedWindowCapture{}
				if c.retained != nil {
					floor, hash = c.retainedBlock, c.retainedHash
					replay = nil
				}
			}
			c.mu.Unlock()
			busy, pending = true, false
			readCtx, stop := context.WithTimeout(ctx, 30*time.Second)
			captureCancel = stop
			captureEpoch := epoch
			attempt := capture
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer stop()
				s, err := c.resumeRetainedWindow(readCtx, floor, hash, attempt)
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
				if capture != nil && capture.snapshot == nil {
					capture = nil
				}
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
			capture = nil
			candidate := &StateCache{fee: c.fee, retained: result.snapshot, retainedBlock: result.snapshot.header.Number, retainedHash: result.snapshot.header.Hash}
			var replayErr error
			for _, log := range replay {
				apply := candidate.applyRetainedLog
				if len(log.log.Topics) > 0 && log.log.Topics[0] == crypto.Keccak256Hash([]byte("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)")) {
					apply = candidate.applyLiveRetainedLog
				}
				if err := apply(log.log, log.at); err != nil {
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
			if log.Address != c.pool || len(log.Topics) < 2 || log.Topics[1] != c.poolID {
				continue
			}
			if events != nil {
				observedBlock = max(observedBlock, log.BlockNumber)
				duplicate, err := order.accept(log)
				if duplicate {
					continue
				}
				if !log.Removed && log.BlockHash != (common.Hash{}) && events.OnSwapLog != nil && log.Topics[0] == crypto.Keccak256Hash([]byte("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)")) {
					if err := events.OnSwapLog(ctx, log, receivedAt); err != nil {
						return fmt.Errorf("failed to consume retained live swap: %w", err)
					}
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
			if log.Removed {
				if events != nil && events.OnRemovedLog != nil {
					if err := events.OnRemovedLog(ctx, log, receivedAt); err != nil {
						return fmt.Errorf("failed to consume removed pool log: %w", err)
					}
				}
				continue
			}
			c.mu.Lock()
			missingState := c.retained == nil
			c.mu.Unlock()
			if busy || capture != nil || (events != nil && missingState) {
				if len(replay) >= retainedReplayLimit {
					err := fmt.Errorf("failed to buffer retained logs: replay=too_long max_length=%d", retainedReplayLimit)
					if events == nil {
						return err
					}
					epoch++
					capture = nil
					if captureCancel != nil {
						captureCancel()
					}
					replay = nil
					c.mu.Lock()
					pending = c.retainedNeedsCapture(amount, baseIsToken0)
					c.mu.Unlock()
					if events.OnRecovery != nil {
						events.OnRecovery(err)
					}
					timer.Reset(delay)
				}
				replay = append(replay, receivedLog{log, receivedAt})
			}
			c.mu.Lock()
			if c.retained != nil {
				err := c.applyLiveRetainedLog(log, receivedAt)
				if err != nil {
					c.mu.Unlock()
					if events == nil {
						return err
					}
					// Preserve usable inputs while repairing a missing or inconsistent
					// liquidity delta in the background.
					pending = true
					nextAttempt = time.Now().Add(delay)
					if events.OnRecovery != nil {
						events.OnRecovery(err)
					}
					timer.Reset(delay)
					continue
				}
				pending = pending || c.retainedNeedsCapture(amount, baseIsToken0)
				c.publishQuoteSnapshotLocked()
			}
			c.mu.Unlock()

		}
	}
}
