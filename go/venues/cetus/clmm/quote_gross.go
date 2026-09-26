package clmm

import (
	"context"
	"fmt"
	"math/big"

	"github.com/k4k3ru-hub/onchain/go/quotestate"
)

// QuoteGrossPair calculates fee-free amounts and actual swap fees on detached inputs.
// It never changes the installed snapshot or acquires missing state over RPC.
//
// Version:
//   - 2026-09-26: Added.
func (s *QuoteSnapshot) QuoteGrossPair(ctx context.Context, params QuotePairParams) (quotestate.GrossPair, error) {
	if s == nil || ctx == nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: dependency=null")
	}
	net, err := s.QuotePair(ctx, params)
	if err != nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: %w", err)
	}
	frozen := s.cache.CaptureQuoteSnapshot()
	if frozen == nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: snapshot=null")
	}
	frozen.cache.retained.snapshot.Pool.FeeRate = 0
	gross, err := frozen.QuotePair(ctx, params)
	if err != nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: %w", err)
	}
	return quotestate.GrossPair{BidAmountOut: new(big.Int).SetUint64(gross.Bid.AmountOut), AskAmountIn: new(big.Int).SetUint64(gross.Ask.AmountIn), BidFeeAmount: new(big.Int).SetUint64(net.Bid.FeeAmount), AskFeeAmount: new(big.Int).SetUint64(net.Ask.FeeAmount)}, nil
}

// QuoteGrossExactInput calculates fee-free output and normal input fees on frozen state.
// Missing coverage returns an error without acquiring state over RPC.
//
// Version:
//   - 2026-09-26: Added.
func (s *QuoteSnapshot) QuoteGrossExactInput(ctx context.Context, p QuoteExactInputParams) (quotestate.GrossExactInput, error) {
	if s == nil || ctx == nil {
		return quotestate.GrossExactInput{}, fmt.Errorf("failed to quote gross exact input: dependency=null")
	}
	if p.Pool.Address != s.cache.pool {
		return quotestate.GrossExactInput{}, fmt.Errorf("failed to quote gross exact input: pool=invalid")
	}
	if err := ctx.Err(); err != nil {
		return quotestate.GrossExactInput{}, fmt.Errorf("failed to quote gross exact input: %w", err)
	}
	frozen := s.cache.CaptureQuoteSnapshot()
	if frozen == nil {
		return quotestate.GrossExactInput{}, fmt.Errorf("failed to quote gross exact input: snapshot=null")
	}
	net, err := frozen.cache.retained.snapshot.Quote(p.AmountIn, p.A2B, true)
	if err != nil {
		return quotestate.GrossExactInput{}, fmt.Errorf("failed to quote gross exact input: %w", err)
	}
	frozen.cache.retained.snapshot.Pool.FeeRate = 0
	gross, err := frozen.cache.retained.snapshot.Quote(p.AmountIn, p.A2B, true)
	if err != nil {
		return quotestate.GrossExactInput{}, fmt.Errorf("failed to quote gross exact input: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return quotestate.GrossExactInput{}, fmt.Errorf("failed to quote gross exact input: %w", err)
	}
	return quotestate.GrossExactInput{AmountOut: new(big.Int).SetUint64(gross.AmountOut), FeeAmount: new(big.Int).SetUint64(net.FeeAmount)}, nil
}
