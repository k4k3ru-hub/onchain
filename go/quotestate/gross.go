package quotestate

import "math/big"

// GrossPair separates fee-free price impact from fees calculated with normal settings.
// Amounts are minimum units: BidAmountOut/AskAmountIn are Quote quantities;
// BidFeeAmount is Base and AskFeeAmount is Quote. Fees include the protocol share.
// The two simulations need not traverse identical price paths, so subtracting a
// converted fee from a gross amount does not reconstruct a net quote.
type GrossPair struct {
	BidAmountOut, AskAmountIn  *big.Int
	BidFeeAmount, AskFeeAmount *big.Int
}

// GrossExactInput contains fee-free output and the normal fee charged in input units.
// Both simulations spend the same exact input against the same frozen state.
type GrossExactInput struct {
	AmountOut *big.Int
	FeeAmount *big.Int
}
