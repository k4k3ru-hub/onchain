package slipstream

import (
	"context"
	"fmt"
	"math/big"
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

// RunRetained subscribes before initialization and maintains local quote inputs.
// RPC reads occur only during initialization. State notifications never publish
// quotes. On failure all retained inputs are discarded; the caller may reconnect.
//
// Version:
//   - 2026-09-09: Publish retained input updates and withdraw unavailable state.
//   - 2026-09-09: Added.
func (c *StateCache) RunRetained(ctx context.Context, ws WSRPCClient, amount *big.Int, baseIsToken0 bool) error {
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
	c.running, c.retained = true, nil
	c.publishQuoteSnapshotLocked()
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.running, c.retained = false, nil; c.publishQuoteSnapshotLocked(); c.mu.Unlock() }()
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
	sub, err := ws.SubscribeFilterLogs(ctx, ethereum.FilterQuery{Addresses: []common.Address{c.pool, c.factory, module}}, logs)
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
	state, err := c.initializeRetainedPool(initCtx, amount, baseIsToken0)
	if err != nil {
		return err
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
	var pending []types.Log
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
			err := c.applyRetainedLog(log, timestamp)
			c.publishQuoteSnapshotLocked()
			c.mu.Unlock()
			if err != nil {
				return err
			}
			pending[0] = types.Log{}
			pending = pending[1:]
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to run retained slipstream state: %w", ctx.Err())
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
			if log.Removed || log.BlockHash == (common.Hash{}) {
				return fmt.Errorf("failed to receive retained logs: event=invalid")
			}
			if len(pending) >= 4096 {
				return fmt.Errorf("failed to retain pending logs: buffer=too_long")
			}
			pending = append(pending, log)
		}
		if err := apply(); err != nil {
			return err
		}
	}
}

func (c *StateCache) initializeRetainedPool(ctx context.Context, amount *big.Int, baseIsToken0 bool) (*retainedPoolState, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("failed to initialize retained pool: %w", ctx.Err())
	case <-c.gate:
	}
	defer func() { c.gate <- struct{}{} }()
	budget := 64
	snapshot, err := c.capture(ctx, &budget, 0, common.Hash{})
	if err != nil {
		return nil, err
	}
	if _, err := c.quote(ctx, snapshot, amount, baseIsToken0, true, &budget); err != nil {
		return nil, err
	}
	if _, err := c.quote(ctx, snapshot, amount, !baseIsToken0, false, &budget); err != nil {
		return nil, err
	}
	fee, err := c.captureRetainedFeeState(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	return &retainedPoolState{pool: snapshot, fee: fee, timestamp: snapshot.header.Timestamp}, nil
}
