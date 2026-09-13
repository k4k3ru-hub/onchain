package cpmm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"github.com/k4k3ru-hub/onchain/go/solana/internal/programevents"
)

// DecodeSwapLog decodes individual committed Swap events without fetching transactions.
// Amounts represent pool vault movements; EventIndex is the position in the received logs.
//
// Returns:
//   - Validated Swap events, including the completed prefix when logs are truncated.
//   - ErrExecutionLogsTruncated with partial results, or a fatal error with no events.
//
// Version:
//   - 2026-09-13: Preserve completed Swap events alongside an inspectable truncation error.
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
	data, logErr := programevents.Read(log, program)
	if logErr != nil {
		logErr = fmt.Errorf("failed to decode raydium swap log: %w", logErr)
		if !errors.Is(logErr, solana.ErrExecutionLogsTruncated) {
			return nil, logErr
		}
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
		if len(b) != 153 && len(b) != 170 {
			return nil, fmt.Errorf("failed to decode raydium swap event: data=invalid actual_length=%d", len(b))
		}
		if b[88] > 1 {
			return nil, fmt.Errorf("failed to decode raydium swap event: base_input=invalid")
		}
		copy(inMint[:], b[89:121])
		copy(outMint[:], b[121:153])
		if !((inMint == pool.Token0Mint && outMint == pool.Token1Mint) || (inMint == pool.Token1Mint && outMint == pool.Token0Mint)) {
			return nil, fmt.Errorf("failed to decode raydium swap event: mints=invalid")
		}
		amountIn, amountOut = binary.LittleEndian.Uint64(b[56:64]), binary.LittleEndian.Uint64(b[64:72])
		if amountIn == 0 || amountOut == 0 {
			return nil, fmt.Errorf("failed to decode raydium swap event: amount=empty")
		}
		result = append(result, &SwapEvent{Signature: log.Signature, Slot: log.Slot, Pool: poolAddress, InputMint: inMint, OutputMint: outMint, AmountIn: amountIn, AmountOut: amountOut, EventIndex: item.Index})
	}
	return result, logErr
}
