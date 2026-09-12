package clmm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"github.com/k4k3ru-hub/onchain/go/solana/internal/programevents"
	"math"
)

// DecodeSwapLog decodes individual committed Swap events without fetching transactions.
// Amounts represent pool vault movements; EventIndex is the position in the received logs.
//
// Version:
//   - 2026-09-12: Added.
func (s *SwapSubscriber) DecodeSwapLog(poolAddress, program solana.Address, log *solana.Log) ([]*SwapEvent, error) {
	if s == nil {
		return nil, fmt.Errorf("failed to decode raydium swap log: subscriber=null")
	}
	pool, ok := s.pools[poolAddress]
	if !ok {
		return nil, fmt.Errorf("failed to decode raydium swap log: pool=invalid")
	}
	if program.IsZero() {
		program = MainnetProgramAddress()
	}
	data, err := programevents.Read(log, program)
	if err != nil {
		return nil, err
	}
	var result []*SwapEvent
	for _, item := range data {
		b := item.Data
		if len(b) < 8 || !bytes.Equal(b[:8], []byte{64, 198, 205, 232, 38, 8, 113, 226}) {
			continue
		}
		if len(b) < 40 {
			return nil, fmt.Errorf("failed to decode raydium swap event: data=too_short")
		}
		var address solana.Address
		copy(address[:], b[8:40])
		if address != poolAddress {
			continue
		}
		var inMint, outMint solana.Address
		var amountIn, amountOut uint64
		if len(b) != 205 && len(b) != 221 {
			return nil, fmt.Errorf("failed to decode raydium swap event: data=invalid actual_length=%d", len(b))
		}
		if b[168] > 1 {
			return nil, fmt.Errorf("failed to decode raydium swap event: direction=invalid")
		}
		a0, a1 := binary.LittleEndian.Uint64(b[136:144]), binary.LittleEndian.Uint64(b[152:160])
		fee0, fee1 := binary.LittleEndian.Uint64(b[144:152]), binary.LittleEndian.Uint64(b[160:168])
		var outputFee uint64
		if b[168] == 1 {
			inMint, outMint = pool.Token0Mint, pool.Token1Mint
			amountIn, amountOut, outputFee = a0, a1, fee1
		} else {
			inMint, outMint = pool.Token1Mint, pool.Token0Mint
			amountIn, amountOut, outputFee = a1, a0, fee0
		}
		if amountOut > math.MaxUint64-outputFee {
			return nil, fmt.Errorf("failed to decode raydium swap event: amount=out_of_range")
		}
		amountOut += outputFee
		if amountIn == 0 || amountOut == 0 {
			return nil, fmt.Errorf("failed to decode raydium swap event: amount=empty")
		}
		result = append(result, &SwapEvent{Signature: log.Signature, Slot: log.Slot, Pool: poolAddress, InputMint: inMint, OutputMint: outMint, AmountIn: amountIn, AmountOut: amountOut, EventIndex: item.Index})
	}
	return result, nil
}
