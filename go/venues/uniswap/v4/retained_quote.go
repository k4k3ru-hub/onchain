package v4

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
// Removed logs or unsupported state deltas terminate the session for reinitialization.
// This state producer never publishes a quote.
//
// Version:
//   - 2026-09-10: Refresh coverage asynchronously without restarting the subscription.
//   - 2026-09-09: Publish retained input updates and withdraw unavailable state.
//   - 2026-09-09: Added.
func (c *StateCache) RunRetained(ctx context.Context, ws WSRPCClient, baseAmount *big.Int, baseIsToken0 bool) error {
	if c == nil {
		return fmt.Errorf("failed to run retained state: cache=null")
	}
	if baseAmount == nil || baseAmount.Sign() <= 0 || baseAmount.BitLen() > 128 {
		return fmt.Errorf("failed to run retained state: amount=out_of_range")
	}
	return c.runRetainedSubscription(ctx, ws, new(big.Int).Set(baseAmount), baseIsToken0)
}

// QuoteRetainedPair calculates from a detached copy of currently retained inputs.
// It does not wait for Trade positions, refresh state or fetch missing ticks.
// BlockNumber/BlockHash/ObservedAt describe the initial verified input baseline;
// streamed core fields may be newer and do not imply an atomic on-chain snapshot.
//
// Version:
//   - 2026-09-10: Request asynchronous capture for missing reference-quote inputs.
//   - 2026-09-09: Keep local capture time separate from input provenance.
func (c *StateCache) QuoteRetainedPair(ctx context.Context, amount *big.Int, baseIsToken0 bool) (LocalPair, error) {
	if c == nil || ctx == nil {
		return LocalPair{}, fmt.Errorf("failed to quote retained state: dependency=null")
	}
	if amount == nil || amount.Sign() <= 0 || amount.BitLen() > 128 {
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
	return LocalPair{ReceivedAt: snapshot.received, CapturedAt: capturedAt, BidAmountOut: bid, AskAmountIn: ask, BlockNumber: snapshot.header.Number, BlockHash: snapshot.header.Hash, ObservedAt: snapshot.observed}, nil
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
func (c *StateCache) applyRetainedLog(log types.Log, receipt ...time.Time) error {
	fail := func() error {
		c.retained = nil
		return fmt.Errorf("failed to apply retained pool log: state requires reinitialization")
	}
	if log.Removed || log.BlockHash == (common.Hash{}) {
		return fail()
	}
	if c.retained == nil {
		return fail()
	}
	baseline := c.retained.header
	if log.BlockNumber < baseline.Number {
		return nil
	}
	if log.BlockNumber == baseline.Number {
		if log.BlockHash != baseline.Hash {
			return fail()
		}
		return nil
	}
	if c.retainedHasLog && (log.BlockNumber < c.retainedBlock || log.BlockNumber == c.retainedBlock && (log.BlockHash != c.retainedHash || log.Index <= c.retainedIndex)) {
		return fail()
	}
	if len(log.Topics) == 0 {
		return fail()
	}
	switch log.Topics[0] {
	case crypto.Keccak256Hash([]byte("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)")):
		if len(log.Topics) != 3 || len(log.Data) != 192 {
			return fail()
		}
		price, liquidity := new(big.Int).SetBytes(log.Data[64:96]), new(big.Int).SetBytes(log.Data[96:128])
		tick, fee := signedWord(log.Data[128:160]), new(big.Int).SetBytes(log.Data[160:192])
		if price.Cmp(sqrtAtTick(-887272)) < 0 || price.Cmp(sqrtAtTick(887272)) >= 0 || liquidity.BitLen() > 128 || !tick.IsInt64() || tick.Int64() < -887272 || tick.Int64() > 887272 || !fee.IsUint64() {
			return fail()
		}
		if fee.Uint64() != uint64(c.retained.fees[0]) && fee.Uint64() != uint64(c.retained.fees[1]) {
			return fail()
		}
		c.retained.price, c.retained.liquidity, c.retained.tick = price, liquidity, int32(tick.Int64())
	case crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)")):
		if err := applyRetainedLiquidity(c.retained, log); err != nil {
			c.retained = nil
			return fmt.Errorf("failed to apply retained liquidity: %w", err)
		}
	case crypto.Keccak256Hash([]byte("ProtocolFeeUpdated(bytes32,uint24)")):
		if len(log.Topics) != 2 || len(log.Data) != 32 {
			return fail()
		}
		packed := new(big.Int).SetBytes(log.Data)
		if packed.BitLen() > 24 {
			return fail()
		}
		for i, p := range []uint32{uint32(packed.Uint64()) & 0xfff, uint32(packed.Uint64()) >> 12} {
			if p > 1000 {
				return fail()
			}
			c.retained.fees[i] = p + c.fee - uint32(uint64(p)*uint64(c.fee)/1000000)
		}
	case crypto.Keccak256Hash([]byte("Donate(bytes32,address,uint256,uint256)")):
		if len(log.Topics) != 3 || len(log.Data) != 64 {
			return fail()
		}
	default:
		return fail()
	}

	at := time.Now().UTC()
	if len(receipt) > 0 {
		at = receipt[0]
	}
	if at.After(c.retained.received) {
		c.retained.received = at
	}
	c.retainedBlock, c.retainedHash, c.retainedIndex, c.retainedHasLog = log.BlockNumber, log.BlockHash, log.Index, true
	return nil
}

func applyRetainedLiquidity(s *poolSnapshot, log types.Log) error {
	if len(log.Topics) != 3 || len(log.Data) != 128 {
		return fmt.Errorf("failed to decode liquidity delta: event=invalid")
	}
	lowerValue, upperValue := signedWord(log.Data[:32]), signedWord(log.Data[32:64])
	delta := signedWord(log.Data[64:96])
	if !lowerValue.IsInt64() || !upperValue.IsInt64() || lowerValue.Int64() < -887272 || upperValue.Int64() > 887272 || lowerValue.Cmp(upperValue) >= 0 || s.spacing <= 0 {
		return fmt.Errorf("failed to decode liquidity delta: tick_range=invalid")
	}
	lower, upper := int32(lowerValue.Int64()), int32(upperValue.Int64())
	if lower%s.spacing != 0 || upper%s.spacing != 0 || delta.Cmp(new(big.Int).Neg(power2(127))) < 0 || delta.Cmp(power2(127)) >= 0 {
		return fmt.Errorf("failed to decode liquidity delta: range=invalid")
	}
	if delta.Sign() == 0 {
		return nil
	}
	mint := delta.Sign() > 0
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
