package clmm

import (
	"fmt"
	"math/big"

	"github.com/k4k3ru-hub/onchain/go/internal/suiswap"
	"github.com/k4k3ru-hub/onchain/go/sui"
)

type SwapTransactionParams struct {
	Pool                       Pool
	Sender, Recipient          sui.Address
	A2B                        bool
	AmountIn, MinimumAmountOut uint64
	// AmountOut and MaximumAmountIn select exact output; input-mode amounts must be zero.
	AmountOut, MaximumAmountIn           uint64
	InputCoins, GasCoins                 []sui.Coin
	GasPrice, GasBudget, ExpirationEpoch uint64
	DeadlineMS                           uint64
}

// BuildSwapTransaction builds a funded swap with enforced input and output limits.
// AmountIn and MinimumAmountOut select exact input with full input consumption.
// AmountOut and MaximumAmountIn select exact output with full output enforcement.
// Unused input remains with or is refunded to Sender, independently of Recipient.
// The router deadline is in milliseconds.
//
// Version:
//   - 2026-10-02: Support capped exact-output swaps and sender refunds.
//   - 2026-09-28: Added.
func BuildSwapTransaction(deployment Deployment, p SwapTransactionParams) (sui.TransactionData, error) {
	if err := deployment.Validate(); err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build turbos swap transaction: %w", err)
	}
	typeIn, typeOut := p.Pool.CoinTypeA, p.Pool.CoinTypeB
	if !p.A2B {
		typeIn, typeOut = typeOut, typeIn
	}
	tx, err := suiswap.Build(suiswap.Params{Sender: p.Sender, Recipient: p.Recipient, CoinTypeIn: typeIn, CoinTypeOut: typeOut, AmountIn: p.AmountIn, MinimumAmountOut: p.MinimumAmountOut, AmountOut: p.AmountOut, MaximumAmountIn: p.MaximumAmountIn, InputCoins: p.InputCoins, GasCoins: p.GasCoins, GasPrice: p.GasPrice, GasBudget: p.GasBudget, ExpirationEpoch: p.ExpirationEpoch}, func(b *sui.ProgrammableTransactionBuilder, coin, _ sui.Argument) (sui.Argument, sui.Argument, error) {
		limit := new(big.Int).SetUint64(4295048016)
		if !p.A2B {
			var ok bool
			limit, ok = new(big.Int).SetString("79226673515401279992447579055", 10)
			if !ok {
				return sui.Argument{}, sui.Argument{}, fmt.Errorf("failed to build swap transaction: price_limit=invalid")
			}
		}
		specified, threshold := p.AmountIn, p.MinimumAmountOut
		if p.AmountOut != 0 {
			specified, threshold = p.AmountOut, p.MaximumAmountIn
		}
		coins, err := appendSwap(b, deployment, swapParams{Pool: p.Pool, InputCoin: coin, SqrtPriceLimit: limit, A2B: p.A2B, Recipient: p.Recipient, DeadlineMS: p.DeadlineMS, Amount: specified, AmountLimit: threshold, ByAmountIn: p.AmountOut == 0})
		if err != nil {
			return sui.Argument{}, sui.Argument{}, err
		}
		inputCoin, outputCoin := coins.CoinA, coins.CoinB
		if !p.A2B {
			inputCoin, outputCoin = outputCoin, inputCoin
		}
		input, err := sui.AppendCoinIntoBalance(b, typeIn, inputCoin)
		if err != nil {
			return sui.Argument{}, sui.Argument{}, err
		}
		output, err := sui.AppendCoinIntoBalance(b, typeOut, outputCoin)
		return input, output, err
	})
	if err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build turbos swap transaction: %w", err)
	}
	return tx, nil
}
