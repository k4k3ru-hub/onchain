package v3

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var errStateReadBudget = errors.New("rpc_budget=exhausted")

// RunRetained subscribes and maintains a three-word window around the current tick.
// Removed logs or unsupported state deltas terminate the session with a reason for reinitialization.
// This state producer never publishes a quote.
//
// Version:
//   - 2026-09-10: Identify reinitialization reasons and refresh coverage asynchronously.
//   - 2026-09-09: Publish retained input updates and withdraw unavailable state.
//   - 2026-09-09: Recover missing reference-quote coverage through the state session.
func (c *StateCache) RunRetained(ctx context.Context, ws WSRPCClient, baseAmount *big.Int, baseIsToken0 bool) error {
	if c == nil {
		return fmt.Errorf("failed to run retained state: cache=null")
	}
	if baseAmount == nil || baseAmount.Sign() <= 0 || baseAmount.BitLen() > 255 {
		return fmt.Errorf("failed to run retained state: amount=invalid")
	}
	return c.runRetainedSubscription(ctx, ws, new(big.Int).Set(baseAmount), baseIsToken0)
}

// QuoteRetainedPair calculates from a detached copy of currently retained inputs.
// It does not wait for Trade positions, refresh state or fetch missing ticks.
// BlockNumber/BlockHash/ObservedAt describe the initial verified input baseline;
// streamed core fields may be newer and do not imply an atomic on-chain snapshot.
//
// Version:
//   - 2026-09-09: Notify the state producer of missing reference-quote coverage without waiting.
//   - 2026-09-09: Keep local capture time separate from input provenance.
func (c *StateCache) QuoteRetainedPair(ctx context.Context, amount *big.Int, baseIsToken0 bool) (LocalPair, error) {
	if c == nil || ctx == nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained state: dependency=null")
	}
	if amount == nil || amount.Sign() <= 0 || amount.BitLen() > 255 {
		return LocalPair{}, fmt.Errorf("failed to quote retained state: amount=out_of_range")
	}
	if err := ctx.Err(); err != nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained state: %w", err)
	}
	c.mu.Lock()
	capturedAt := time.Now()
	source := c.retained
	snapshot := clonePoolSnapshot(source)
	c.mu.Unlock()
	if snapshot == nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained state: snapshot=null")
	}
	budget := 0
	bid, err := c.quote(ctx, snapshot, amount, baseIsToken0, true, &budget)
	if err != nil {
		c.requestRetainedRecovery(source, amount, err)
		return LocalPair{}, fmt.Errorf("failed to quote retained bid: %w", err)
	}
	ask, err := c.quote(ctx, snapshot, amount, !baseIsToken0, false, &budget)
	if err != nil {
		c.requestRetainedRecovery(source, amount, err)
		return LocalPair{}, fmt.Errorf("failed to quote retained ask: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained state: %w", err)
	}
	return LocalPair{CapturedAt: capturedAt, BidAmountOut: bid, AskAmountIn: ask, BlockNumber: snapshot.header.Number, BlockHash: snapshot.header.Hash, ObservedAt: snapshot.observed}, nil
}

func clonePoolSnapshot(source *poolSnapshot) *poolSnapshot {
	if source == nil {
		return nil
	}
	result := *source
	result.price = new(big.Int).Set(source.price)
	result.liquidity = new(big.Int).Set(source.liquidity)
	result.words = cloneRetainedIntegers(source.words)
	result.ticks = cloneRetainedIntegers(source.ticks)
	result.gross = cloneRetainedIntegers(source.gross)
	return &result
}
func cloneRetainedIntegers(source map[int32]*big.Int) map[int32]*big.Int {
	result := make(map[int32]*big.Int, len(source))
	for key, value := range source {
		if value != nil {
			result[key] = new(big.Int).Set(value)
		}
	}
	return result
}

// applyRetainedLog runs under mu. Events before the bootstrap block are already
// incorporated. Within subsequent blocks, reception must follow log order.
func (c *StateCache) applyRetainedLog(log types.Log) error {
	fail := func(reason string) error {
		c.retained = nil
		return retainedReinitializationError(log, reason)
	}
	if log.Removed {
		return fail("removed log")
	}
	if log.BlockHash == (common.Hash{}) {
		return fail("missing block hash")
	}
	if c.retained == nil {
		return fail("missing snapshot")
	}
	baseline := c.retained.header
	if log.BlockNumber < baseline.Number {
		return nil
	}
	if log.BlockNumber == baseline.Number {
		if log.BlockHash != baseline.Hash {
			return fail("baseline hash mismatch")
		}
		return nil
	}
	if c.retainedHasLog {
		if log.BlockNumber < c.retainedBlock {
			return fail("block order regression")
		}
		if log.BlockNumber == c.retainedBlock {
			if log.BlockHash != c.retainedHash {
				return fail("stream block hash mismatch")
			}
			if log.Index <= c.retainedIndex {
				return fail("duplicate or out of order log")
			}
		}
	}
	if len(log.Topics) == 0 {
		return fail("missing event topics")
	}
	switch log.Topics[0] {
	case swapEventSignatureHash():
		swap, err := DecodeSwapLog(log)
		if err != nil {
			c.retained = nil
			return fmt.Errorf("failed to apply retained swap: %w", err)
		}
		c.retained.price, c.retained.liquidity, c.retained.tick = swap.SqrtPriceX96, swap.Liquidity, swap.Tick
	case crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)")), crypto.Keccak256Hash([]byte("Burn(address,int24,int24,uint128,uint256,uint256)")):
		if err := applyRetainedLiquidity(c.retained, log); err != nil {
			c.retained = nil
			return fmt.Errorf("failed to apply retained liquidity: %w", err)
		}
	case crypto.Keccak256Hash([]byte("Collect(address,address,int24,int24,uint128,uint128)")),
		crypto.Keccak256Hash([]byte("CollectProtocol(address,address,uint128,uint128)")),
		crypto.Keccak256Hash([]byte("SetFeeProtocol(uint8,uint8,uint8,uint8)")),
		crypto.Keccak256Hash([]byte("IncreaseObservationCardinalityNext(uint16,uint16)")),
		crypto.Keccak256Hash([]byte("Flash(address,address,uint256,uint256,uint256,uint256)")):
		// These do not change executable liquidity, price, tick or swap fee.
	default:
		// Unknown events require producer recovery before applying further deltas.
		return fail("unsupported event")
	}
	c.retainedBlock, c.retainedHash, c.retainedIndex, c.retainedHasLog = log.BlockNumber, log.BlockHash, log.Index, true
	return nil
}

func retainedReinitializationError(log types.Log, reason string) error {
	return fmt.Errorf("failed to apply retained pool log: state requires reinitialization: reason=%q pool=%q block_number=%d block_hash=%q log_index=%d", reason, log.Address.Hex(), log.BlockNumber, log.BlockHash.Hex(), log.Index)
}

func applyRetainedLiquidity(s *poolSnapshot, log types.Log) error {
	mint := log.Topics[0] == crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)"))
	offset, size := 0, 96
	if mint {
		offset, size = 32, 128
	}
	if len(log.Topics) != 4 || len(log.Data) != size {
		return fmt.Errorf("failed to decode liquidity delta: event=invalid")
	}
	lowerWord, upperWord := log.Topics[2], log.Topics[3]
	if !hasCanonicalSignedPadding(lowerWord[:], 24) || !hasCanonicalSignedPadding(upperWord[:], 24) || !hasCanonicalUnsignedPadding(log.Data[offset:offset+32], 128) {
		return fmt.Errorf("failed to decode liquidity delta: padding=invalid")
	}
	signedTick := func(word common.Hash) int32 {
		value := new(big.Int).SetBytes(word[:])
		if value.Bit(255) != 0 {
			value.Sub(value, power2(256))
		}
		return int32(value.Int64())
	}
	lower, upper := signedTick(lowerWord), signedTick(upperWord)
	if s.spacing <= 0 || lower >= upper || lower < -887272 || upper > 887272 || lower%s.spacing != 0 || upper%s.spacing != 0 {
		return fmt.Errorf("failed to decode liquidity delta: tick_range=invalid")
	}
	amount := new(big.Int).SetBytes(log.Data[offset : offset+32])
	if amount.Sign() == 0 {
		return nil
	}
	delta := new(big.Int).Set(amount)
	if !mint {
		delta.Neg(delta)
	}
	if s.tick >= lower && s.tick < upper {
		liquidity := new(big.Int).Add(s.liquidity, delta)
		if liquidity.Sign() < 0 || liquidity.BitLen() > 128 {
			return fmt.Errorf("failed to apply liquidity delta: liquidity=out_of_range")
		}
		s.liquidity = liquidity
	}
	if s.gross == nil {
		s.gross = make(map[int32]*big.Int)
	}
	for _, tick := range []int32{lower, upper} {
		compressed := tick / s.spacing
		wordIndex := compressed >> 8
		bit := int(uint8(compressed))
		word := s.words[wordIndex]
		gross, net := s.gross[tick], s.ticks[tick]
		if word != nil && word.Bit(bit) == 0 {
			if !mint {
				return fmt.Errorf("failed to apply liquidity delta: tick=uninitialized")
			}
			gross, net = new(big.Int), new(big.Int)
		}
		if gross != nil && net != nil {
			gross = new(big.Int).Add(gross, delta)
			if gross.Sign() < 0 || gross.BitLen() > 128 {
				return fmt.Errorf("failed to apply liquidity delta: liquidity_gross=out_of_range")
			}
			net = new(big.Int).Set(net)
			if tick == lower {
				net.Add(net, delta)
			} else {
				net.Sub(net, delta)
			}
			if net.Cmp(new(big.Int).Neg(power2(127))) < 0 || net.Cmp(power2(127)) >= 0 {
				return fmt.Errorf("failed to apply liquidity delta: liquidity_net=out_of_range")
			}
			s.gross[tick], s.ticks[tick] = gross, net
			if word != nil {
				if gross.Sign() == 0 {
					word.SetBit(word, bit, 0)
				} else {
					word.SetBit(word, bit, 1)
				}
			}
		} else {
			// The word may be known while this tick's net/gross was never needed.
			// Keep its net unavailable: crossing it must skip, never fetch or guess.
			delete(s.ticks, tick)
			delete(s.gross, tick)
			if word != nil && mint {
				word.SetBit(word, bit, 1)
			}
		}
	}
	return nil
}

// Reference-sized failures request recovery once. Larger quantity queries must
// not force baseline captures that cannot cover their requested amount.
func (c *StateCache) requestRetainedRecovery(source *poolSnapshot, amount *big.Int, err error) {
	if !errors.Is(err, errStateReadBudget) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running || c.retained != source || c.retainedBaseAmount == nil || amount.Cmp(c.retainedBaseAmount) > 0 {
		return
	}
	select {
	case c.retainedRecovery <- struct{}{}:
	default:
	}
}
