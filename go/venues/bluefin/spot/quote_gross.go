package spot

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
	frozen.cache.retained.pool.FeeRate = 0
	gross, err := frozen.QuotePair(ctx, params)
	if err != nil {
		return quotestate.GrossPair{}, fmt.Errorf("failed to quote gross pair: %w", err)
	}
	return quotestate.GrossPair{BidAmountOut: new(big.Int).SetUint64(gross.Bid.AmountOut), AskAmountIn: new(big.Int).SetUint64(gross.Ask.AmountIn), BidFeeAmount: new(big.Int).Add(new(big.Int).SetUint64(net.Bid.FeeAmount), new(big.Int).SetUint64(net.Bid.ProtocolFee)), AskFeeAmount: new(big.Int).Add(new(big.Int).SetUint64(net.Ask.FeeAmount), new(big.Int).SetUint64(net.Ask.ProtocolFee))}, nil
}
