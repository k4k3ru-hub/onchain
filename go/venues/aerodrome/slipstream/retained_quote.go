package slipstream

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type retainedPoolState struct {
	pool      *poolSnapshot
	fee       retainedFeeState
	block     uint64
	hash      common.Hash
	index     uint
	hasLog    bool
	timestamp uint64
}

// QuoteRetainedPair calculates from a detached copy of retained pool inputs.
// It never waits for a Trade position or loads missing inputs over RPC.
// BlockNumber, BlockHash and ObservedAt describe the verified initial baseline;
// CapturedAt describes this receiver-side capture, not an atomic chain snapshot.
//
// Version:
//   - 2026-09-09: Added.
func (c *StateCache) QuoteRetainedPair(ctx context.Context, amount *big.Int, baseIsToken0 bool) (LocalPair, error) {
	if c == nil || ctx == nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained slipstream state: dependency=null")
	}
	if amount == nil || amount.Sign() <= 0 || amount.BitLen() > 255 {
		return LocalPair{}, fmt.Errorf("failed to quote retained slipstream state: amount=out_of_range")
	}
	if err := ctx.Err(); err != nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained slipstream state: %w", err)
	}
	c.mu.Lock()
	captured := time.Now().UTC()
	s := cloneRetainedPool(c.retained)
	c.mu.Unlock()
	if s == nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained slipstream state: snapshot=null")
	}
	fee, err := s.fee.oracle.fee(s.fee.config, uint32(s.timestamp), s.pool.tick)
	if err != nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained slipstream state: %w", err)
	}
	s.pool.fee = fee
	budget := 0
	bid, err := c.quote(ctx, s.pool, amount, baseIsToken0, true, &budget)
	if err != nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained slipstream bid: %w", err)
	}
	ask, err := c.quote(ctx, s.pool, amount, !baseIsToken0, false, &budget)
	if err != nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained slipstream ask: %w", err)
	}
	return LocalPair{CapturedAt: captured, BidAmountOut: bid, AskAmountIn: ask, BlockNumber: s.pool.header.Number, BlockHash: s.pool.header.Hash, ObservedAt: s.pool.observed, FeePPM: fee}, nil
}

func cloneRetainedPool(source *retainedPoolState) *retainedPoolState {
	if source == nil || source.pool == nil {
		return nil
	}
	result := *source
	pool := *source.pool
	pool.price = new(big.Int).Set(pool.price)
	pool.liquidity = new(big.Int).Set(pool.liquidity)
	clone := func(values map[int32]*big.Int) map[int32]*big.Int {
		out := make(map[int32]*big.Int, len(values))
		for key, value := range values {
			if value != nil {
				out[key] = new(big.Int).Set(value)
			}
		}
		return out
	}
	pool.words, pool.ticks, pool.gross = clone(pool.words), clone(pool.ticks), clone(pool.gross)
	result.pool = &pool
	result.fee.oracle = source.fee.oracle.clone()
	return &result
}

// applyRetainedLog runs under mu. The producer supplies the block timestamp
// matched by hash. Failure clears every retained input before recovery.
func (c *StateCache) applyRetainedLog(log types.Log, timestamp uint64) error {
	fail := func(err error) error {
		c.retained = nil
		return fmt.Errorf("failed to apply retained slipstream log: %w", err)
	}
	s := c.retained
	if s == nil || s.pool == nil || log.Removed || log.BlockHash == (common.Hash{}) {
		return fail(fmt.Errorf("failed to validate retained event: event=invalid"))
	}
	baseline := s.pool.header
	if log.BlockNumber < baseline.Number {
		return nil
	}
	if log.BlockNumber == baseline.Number {
		if log.BlockHash != baseline.Hash {
			return fail(fmt.Errorf("failed to validate retained event: block=mismatch"))
		}
		return nil
	}
	if len(log.Topics) == 0 || timestamp < s.timestamp || s.hasLog && (log.BlockNumber < s.block || log.BlockNumber == s.block && (log.BlockHash != s.hash || log.Index <= s.index || timestamp != s.timestamp)) {
		return fail(fmt.Errorf("failed to validate retained event: sequence=invalid"))
	}
	// A factory change may replace the fee module; always reconstruct the session.
	if log.Address == c.factory {
		return fail(fmt.Errorf("failed to apply factory event: state requires reinitialization"))
	}
	if log.Address == s.fee.module {
		if err := applyFeeModuleLog(&s.fee.config, c.pool, common.Address{}, log); err != nil {
			return fail(err)
		}
	} else if log.Address == c.pool {
		if err := applyRetainedPoolEvent(s, log, uint32(timestamp)); err != nil {
			return fail(err)
		}
	} else {
		return fail(fmt.Errorf("failed to validate retained event: address=mismatch"))
	}
	s.block, s.hash, s.index, s.hasLog, s.timestamp = log.BlockNumber, log.BlockHash, log.Index, true, timestamp
	return nil
}

func applyRetainedPoolEvent(s *retainedPoolState, log types.Log, timestamp uint32) error {
	hash := func(signature string) common.Hash { return crypto.Keccak256Hash([]byte(signature)) }
	switch log.Topics[0] {
	case swapEventSignatureHash():
		swap, err := DecodeSwapLog(log)
		if err != nil {
			return err
		}
		if swap.SqrtPriceX96.Cmp(sqrtAtTick(-887272)) < 0 || swap.SqrtPriceX96.Cmp(sqrtAtTick(887272)) >= 0 {
			return fmt.Errorf("failed to apply retained swap: price=out_of_range")
		}
		if swap.Tick != s.pool.tick {
			if err := s.fee.oracle.write(timestamp, s.pool.tick); err != nil {
				return err
			}
		}
		s.pool.price, s.pool.liquidity, s.pool.tick = swap.SqrtPriceX96, swap.Liquidity, swap.Tick
	case hash("Mint(address,address,int24,int24,uint128,uint256,uint256)"), hash("Burn(address,int24,int24,uint128,uint256,uint256)"):
		before := new(big.Int).Set(s.pool.liquidity)
		if err := applyRetainedLiquidity(s.pool, log); err != nil {
			return err
		}
		if before.Cmp(s.pool.liquidity) != 0 {
			if err := s.fee.oracle.write(timestamp, s.pool.tick); err != nil {
				return err
			}
		}
	case hash("IncreaseObservationCardinalityNext(uint16,uint16)"):
		if len(log.Topics) != 1 || len(log.Data) != 64 {
			return fmt.Errorf("failed to apply oracle growth: event=invalid")
		}
		old, err := decodeUnsignedWord(log.Data[:32], 16, "old_cardinality")
		if err != nil {
			return err
		}
		next, err := decodeUnsignedWord(log.Data[32:], 16, "next_cardinality")
		if err != nil {
			return err
		}
		if old.Uint64() != uint64(s.fee.oracle.next) || next.Uint64() < old.Uint64() {
			return fmt.Errorf("failed to apply oracle growth: cardinality=mismatch")
		}
		s.fee.oracle.next = uint16(next.Uint64())
	case hash("Collect(address,address,int24,int24,uint128,uint128)"), hash("CollectFees(address,uint128,uint128)"), hash("Flash(address,address,uint256,uint256,uint256,uint256)"):
		// These events do not change executable liquidity, tick or oracle inputs.
	default:
		return fmt.Errorf("failed to apply retained pool event: event=unsupported")
	}
	return nil
}
