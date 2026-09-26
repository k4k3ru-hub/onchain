package v3

import (
	"context"
	"fmt"
	"math/big"

	"github.com/k4k3ru-hub/onchain/go/quotestate"
)

// QuoteGrossPair calculates fee-free amounts and normal swap fees from frozen inputs.
// Missing local coverage returns an error; the live cache and recovery are untouched.
//
// Version:
//   - 2026-09-26: Added.
func (s *QuoteSnapshot) QuoteGrossPair(ctx context.Context, amount *big.Int, baseIsToken0 bool) (quotestate.GrossPair, error) {
	if s == nil || ctx == nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: dependency=null")
	}
	if amount == nil || amount.Sign() <= 0 || amount.BitLen() > 255 {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: amount=out_of_range")
	}
	if err := ctx.Err(); err != nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: %w", err)
	}
	s.cache.mu.Lock()
	snapshot := clonePoolSnapshot(s.cache.retained)
	fee := s.cache.fee
	s.cache.mu.Unlock()
	calculator := StateCache{fee: fee}
	if snapshot == nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: snapshot=null")
	}
	bidFee, askFee := new(big.Int), new(big.Int)
	budget := 0
	if _, err := calculator.quote(ctx, snapshot, amount, baseIsToken0, true, &budget, bidFee); err != nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: %w", retainedQuoteError(err))
	}
	if _, err := calculator.quote(ctx, snapshot, amount, !baseIsToken0, false, &budget, askFee); err != nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: %w", retainedQuoteError(err))
	}
	calculator = StateCache{fee: 0}
	budget = 0
	bid, err := calculator.quote(ctx, snapshot, amount, baseIsToken0, true, &budget)
	if err != nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: %w", retainedQuoteError(err))
	}
	ask, err := calculator.quote(ctx, snapshot, amount, !baseIsToken0, false, &budget)
	if err != nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: %w", retainedQuoteError(err))
	}
	return quotestate.GrossPair{BidAmountOut: bid, AskAmountIn: ask, BidFeeAmount: bidFee, AskFeeAmount: askFee}, nil
}
