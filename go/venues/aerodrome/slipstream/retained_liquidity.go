package slipstream

import (
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
)

func applyRetainedLiquidity(s *poolSnapshot, log types.Log) error {
	mint := log.Topics[0] == crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)"))
	offset, size := 0, 96
	if mint {
		offset, size = 32, 128
	}
	if len(log.Topics) != 4 || len(log.Data) != size {
		return fmt.Errorf("failed to decode liquidity delta: event=invalid")
	}
	lower, err := decodeInt24Word(log.Topics[2][:], "lower_tick")
	if err != nil {
		return err
	}
	upper, err := decodeInt24Word(log.Topics[3][:], "upper_tick")
	if err != nil {
		return err
	}
	if _, err := decodeUnsignedWord(log.Data[offset:offset+32], 128, "liquidity"); err != nil {
		return err
	}
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
