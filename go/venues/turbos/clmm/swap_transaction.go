package clmm

import (
	"fmt"
	"math/big"

	"github.com/k4k3ru-hub/onchain/go/internal/suiswap"
	"github.com/k4k3ru-hub/onchain/go/sui"
)

type SwapTransactionParams struct {
	Pool                                 Pool
	Sender, Recipient                    sui.Address
	A2B                                  bool
	AmountIn, MinimumAmountOut           uint64
	InputCoins, GasCoins                 []sui.Coin
	GasPrice, GasBudget, ExpirationEpoch uint64
	DeadlineMS                           uint64
}

// BuildSwapTransaction builds a funded exact-input swap with an onchain output guard.
// The router deadline is in milliseconds. Unspent input aborts the transaction.
//
// Version:
//   - 2026-09-28: Added.
func BuildSwapTransaction(deployment Deployment, p SwapTransactionParams) (sui.TransactionData, error) {
	if err := deployment.Validate(); err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build turbos swap transaction: %w", err)
	}
	typeIn, typeOut := p.Pool.CoinTypeA, p.Pool.CoinTypeB
	if !p.A2B {
		typeIn, typeOut = typeOut, typeIn
	}
	tx, err := suiswap.Build(suiswap.Params{Sender: p.Sender, Recipient: p.Recipient, CoinTypeIn: typeIn, CoinTypeOut: typeOut, AmountIn: p.AmountIn, MinimumAmountOut: p.MinimumAmountOut, InputCoins: p.InputCoins, GasCoins: p.GasCoins, GasPrice: p.GasPrice, GasBudget: p.GasBudget, ExpirationEpoch: p.ExpirationEpoch}, func(b *sui.ProgrammableTransactionBuilder, coin, _ sui.Argument) (sui.Argument, sui.Argument, error) {
		limit := new(big.Int).SetUint64(4295048016)
		if !p.A2B {
			var ok bool
			limit, ok = new(big.Int).SetString("79226673515401279992447579055", 10)
			if !ok {
				return sui.Argument{}, sui.Argument{}, fmt.Errorf("failed to build swap transaction: price_limit=invalid")
			}
		}
		coins, err := AppendSwapExactInput(b, deployment, SwapExactInputParams{Pool: p.Pool, InputCoin: coin, AmountIn: p.AmountIn, MinimumOut: p.MinimumAmountOut, SqrtPriceLimit: limit, A2B: p.A2B, Recipient: p.Recipient, DeadlineMS: p.DeadlineMS})
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
