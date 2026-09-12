package slipstream

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
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type retainedHeaderSubscriber interface {
	SubscribeHeaders(context.Context) (*evm.HeaderSubscription, error)
}

type RunRetainedParams struct {
	Amount       *big.Int
	BaseIsToken0 bool
	// FromBlock is forwarded to the log subscriber; nil preserves its default.
	FromBlock *big.Int
}

// RunRetained subscribes before initialization and maintains local quote inputs.
// RPC reads occur during initialization and background window capture. State notifications never publish
// quotes. Transport reconnection retains existing inputs and their original freshness.
//
// Version:
//   - 2026-09-11: Refresh complete tick windows in the background.
//   - 2026-09-10: Delegate to RunRetainedWithParams using the subscriber's default start block.
//   - 2026-09-09: Publish retained input updates and withdraw unavailable state.
//   - 2026-09-09: Added.
func (c *StateCache) RunRetained(ctx context.Context, ws WSRPCClient, amount *big.Int, baseIsToken0 bool) error {
	return c.RunRetainedWithParams(ctx, ws, RunRetainedParams{Amount: amount, BaseIsToken0: baseIsToken0})
}

// RunRetainedWithParams subscribes before initialization using the requested start block.
// FromBlock controls the subscription filter, not historical replay guarantees.
//
// Parameters:
//   - params: Reference quote amount, direction, and optional subscription start block.
//
// Returns:
//   - Subscription or state initialization error.
//
// Version:
//   - 2026-09-12: Resume failed background tick batches at their original block.
//   - 2026-09-12: Retain inputs across transport reconnects and accept replacement Swaps.
//   - 2026-09-11: Prefetch and refresh complete center ±1 tick windows.
//   - 2026-09-10: Added.
func (c *StateCache) RunRetainedWithParams(ctx context.Context, ws WSRPCClient, params RunRetainedParams) error {
	amount, baseIsToken0 := params.Amount, params.BaseIsToken0
	if c == nil || ctx == nil || ws == nil || amount == nil || amount.Sign() <= 0 || amount.BitLen() > 255 {
		return fmt.Errorf("failed to run retained slipstream state: configuration=invalid")
	}
	headerWS, ok := ws.(retainedHeaderSubscriber)
	if !ok {
		return fmt.Errorf("failed to run retained slipstream state: header_subscription=unsupported")
	}
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("failed to run retained slipstream state: subscription=active")
	}
	c.running = true
	c.publishQuoteSnapshotLocked()
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.running = false; c.publishQuoteSnapshotLocked(); c.mu.Unlock() }()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	initCtx, stopInit := context.WithTimeout(ctx, 30*time.Second)
	defer stopInit()
	// Discover only the address before subscribing, then verify it against the
	// pinned capture. Module replacement during initialization forces recovery.
	data, err := c.rpc.CallContract(initCtx, ethereum.CallMsg{To: &c.factory, Data: crypto.Keccak256([]byte("swapFeeModule()"))[:4]}, nil)
	if err != nil {
		return fmt.Errorf("failed to discover retained fee module: %w", err)
	}
	word, err := decodeUnsignedWord(data, 160, "fee_module")
	if err != nil {
		return err
	}
	module := common.BigToAddress(word)
	if module == (common.Address{}) {
		return fmt.Errorf("failed to discover retained fee module: module=unsupported")
	}
	heads, err := headerWS.SubscribeHeaders(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe retained headers: %w", err)
	}
	defer heads.Close()
	logs := make(chan types.Log, 4096)
	var fromBlock *big.Int
	if params.FromBlock != nil {
		fromBlock = new(big.Int).Set(params.FromBlock)
	}
	sub, err := ws.SubscribeFilterLogs(ctx, ethereum.FilterQuery{
		Addresses: []common.Address{c.pool, c.factory, module},
		FromBlock: fromBlock,
	}, logs)
	if err != nil {
		return fmt.Errorf("failed to subscribe retained logs: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("failed to subscribe retained logs: subscription=null")
	}
	defer sub.Unsubscribe()
	type headResult struct {
		header evm.BlockHeader
		err    error
	}
	headerResults := make(chan headResult, 128)
	go func() {
		for {
			header, err := heads.Recv(ctx)
			select {
			case headerResults <- headResult{header, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	c.mu.Lock()
	state := c.retained
	c.mu.Unlock()
	if state == nil || state.fee.module != module {
		state, err = c.initializeRetainedPool(initCtx, amount, baseIsToken0)
		if err != nil {
			return err
		}
	}
	if state.fee.module != module {
		return fmt.Errorf("failed to initialize retained pool: fee_module=mismatch")
	}
	stopInit()
	c.mu.Lock()
	c.retained = state
	c.publishQuoteSnapshotLocked()
	c.mu.Unlock()
	headers := map[common.Hash]evm.BlockHeader{state.pool.header.Hash: state.pool.header}
	order := []common.Hash{state.pool.header.Hash}
	type receivedLog struct {
		types.Log
		at time.Time
	}
	var pending []receivedLog
	type windowResult struct {
		state *retainedPoolState
		err   error
	}
	results := make(chan windowResult, 1)
	var captures sync.WaitGroup
	defer func() { cancel(); captures.Wait() }()
	busy := false
	var capture *retainedWindowCapture
	var replay []retainedWindowLog
	retryAt := time.Now()
	backoff := time.Second
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	apply := func() error {
		for len(pending) > 0 {
			log := pending[0]
			var timestamp uint64
			if log.BlockNumber > state.pool.header.Number {
				header, ok := headers[log.BlockHash]
				if !ok {
					return nil
				}
				if header.Number != log.BlockNumber {
					return fmt.Errorf("failed to match retained log header: block=mismatch")
				}
				timestamp = header.Timestamp
			}
			c.mu.Lock()
			err := c.applyLiveRetainedLog(log.Log, timestamp, log.at)
			c.publishQuoteSnapshotLocked()
			c.mu.Unlock()
			if err != nil {
				return err
			}
			if busy || capture != nil {
				if len(replay) >= 4096 {
					return fmt.Errorf("failed to retain window replay: buffer=too_long")
				}
				replay = append(replay, retainedWindowLog{log.Log, timestamp, log.at})
			}
			pending[0] = receivedLog{}
			pending = pending[1:]
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to run retained slipstream state: %w", ctx.Err())
		case <-ticker.C:
			c.mu.Lock()
			needed := c.retainedNeedsCapture(amount, baseIsToken0)
			floor, floorHash := uint64(0), common.Hash{}
			if c.retained != nil {
				floor, floorHash = c.retained.pool.header.Number, c.retained.pool.header.Hash
				if c.retained.hasLog {
					floor, floorHash = c.retained.block, c.retained.hash
				}
			}
			c.mu.Unlock()
			if (needed || capture != nil) && !busy && !time.Now().Before(retryAt) {
				fresh := capture == nil
				if fresh {
					capture = &retainedWindowCapture{floor: floor, floorHash: floorHash}
					replay = nil
				}
				attempt := capture
				busy = true
				captures.Add(1)
				go func() {
					defer captures.Done()
					readCtx, stop := context.WithTimeout(ctx, 30*time.Second)
					defer stop()
					next, err := c.resumeRetainedPool(readCtx, attempt)
					if err == nil && (next.pool.header.Number < attempt.floor || next.pool.header.Number == attempt.floor && next.pool.header.Hash != attempt.floorHash) {
						err = fmt.Errorf("failed to capture retained window: baseline=behind_or_conflicting")
						*attempt = retainedWindowCapture{}
					}
					select {
					case results <- windowResult{next, err}:
					case <-ctx.Done():
					}
				}()
			}
		case result := <-results:
			busy = false
			err := result.err
			if err == nil {
				capture = nil
				c.mu.Lock()
				err = c.installRetainedWindow(result.state, replay)
				if err == nil {
					state = c.retained
				}
				c.mu.Unlock()
			}
			if capture != nil && capture.snapshot == nil {
				capture = nil
			}
			if capture == nil {
				replay = nil
			}
			if err == nil {
				backoff = time.Second
			} else {
				backoff = min(2*backoff, 30*time.Second)
			}
			retryAt = time.Now().Add(backoff)
		case err, ok := <-sub.Err():
			if !ok || err == nil {
				return fmt.Errorf("failed to receive retained logs: subscription closed")
			}
			return fmt.Errorf("failed to receive retained logs: %w", err)
		case result := <-headerResults:
			if result.err != nil {
				return fmt.Errorf("failed to receive retained header: %w", result.err)
			}
			header := result.header
			if _, exists := headers[header.Hash]; !exists {
				order = append(order, header.Hash)
			}
			headers[header.Hash] = header
			if len(order) > 256 {
				oldest := order[0]
				for _, log := range pending {
					if log.BlockHash == oldest {
						return fmt.Errorf("failed to retain log headers: pending_header=expired")
					}
				}
				delete(headers, oldest)
				order = order[1:]
			}
		case log, ok := <-logs:
			if !ok {
				return fmt.Errorf("failed to receive retained logs: channel closed")
			}
			if log.Removed {
				continue
			}
			if log.BlockHash == (common.Hash{}) {
				return fmt.Errorf("failed to receive retained logs: event=invalid")
			}
			if len(pending) >= 4096 {
				return fmt.Errorf("failed to retain pending logs: buffer=too_long")
			}
			pending = append(pending, receivedLog{log, time.Now().UTC()})
		}
		if err := apply(); err != nil {
			return err
		}
	}
}

// retainedWindowCapture belongs to one uninterrupted state subscription.
type retainedWindowCapture struct {
	floor       uint64
	floorHash   common.Hash
	snapshot    *poolSnapshot
	bitmapReady bool
	ticks       retainedTickProgress
}

func (c *StateCache) initializeRetainedPool(ctx context.Context, amount *big.Int, baseIsToken0 bool) (*retainedPoolState, error) {
	return c.resumeRetainedPool(ctx, &retainedWindowCapture{})
}

func (c *StateCache) resumeRetainedPool(ctx context.Context, progress *retainedWindowCapture) (*retainedPoolState, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("failed to initialize retained pool: %w", ctx.Err())
	case <-c.gate:
	}
	defer func() { c.gate <- struct{}{} }()
	budget := 64
	if progress.snapshot == nil {
		progress.ticks.started = time.Now()
		snapshot, err := c.capture(ctx, &budget, 0, common.Hash{})
		if err != nil {
			return nil, err
		}
		progress.snapshot = snapshot
	} else {
		header, err := c.rpc.HeaderByNumber(ctx, progress.snapshot.header.Number)
		if err != nil {
			return nil, fmt.Errorf("failed to verify retained window: %w: block_number=%d", err, progress.snapshot.header.Number)
		}
		if header.Number != progress.snapshot.header.Number || header.Hash != progress.snapshot.header.Hash {
			block := progress.snapshot.header.Number
			*progress = retainedWindowCapture{}
			return nil, fmt.Errorf("failed to verify retained window: block_hash=mismatch block_number=%d", block)
		}
	}
	snapshot := progress.snapshot
	center := bitmapWord(snapshot.tick, snapshot.spacing)
	snapshot.windowCenter, snapshot.windowCaptured = center, true
	lower := max(center-retainedWordRadius, bitmapWord(-887272, snapshot.spacing))
	upper := min(center+retainedWordRadius, bitmapWord(887272, snapshot.spacing))
	if !progress.bitmapReady {
		if err := c.readBitmapWindow(ctx, snapshot, lower, upper, &budget); err != nil {
			return nil, err
		}
		progress.bitmapReady = true
	}
	if err := c.resumeWindowTicks(ctx, snapshot, lower, upper, &progress.ticks); err != nil {
		return nil, err
	}
	// Fee/oracle acquisition and its final hash verification use this same baseline.
	fee, err := c.captureRetainedFeeState(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	return &retainedPoolState{received: snapshot.observed, pool: snapshot, fee: fee, timestamp: snapshot.header.Timestamp}, nil
}
