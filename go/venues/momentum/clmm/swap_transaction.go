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
	XForY                                bool
	AmountIn, MinimumAmountOut           uint64
	InputCoins, GasCoins                 []sui.Coin
	GasPrice, GasBudget, ExpirationEpoch uint64
}

// BuildSwapTransaction builds a funded exact-input swap with an onchain output guard.
// Unspent input aborts the transaction; input change remains with the sender.
//
// Version:
//   - 2026-09-28: Added.
func BuildSwapTransaction(deployment Deployment, p SwapTransactionParams) (sui.TransactionData, error) {
	if err := deployment.Validate(); err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build momentum swap transaction: %w", err)
	}
	typeIn, typeOut := p.Pool.CoinTypeX, p.Pool.CoinTypeY
	if !p.XForY {
		typeIn, typeOut = typeOut, typeIn
	}
	tx, err := suiswap.Build(suiswap.Params{Sender: p.Sender, Recipient: p.Recipient, CoinTypeIn: typeIn, CoinTypeOut: typeOut, AmountIn: p.AmountIn, MinimumAmountOut: p.MinimumAmountOut, InputCoins: p.InputCoins, GasCoins: p.GasCoins, GasPrice: p.GasPrice, GasBudget: p.GasBudget, ExpirationEpoch: p.ExpirationEpoch}, func(b *sui.ProgrammableTransactionBuilder, coin, amount sui.Argument) (sui.Argument, sui.Argument, error) {
		input, err := sui.AppendCoinIntoBalance(b, typeIn, coin)
		if err != nil {
			return sui.Argument{}, sui.Argument{}, err
		}
		output, err := sui.AppendZeroBalance(b, typeOut)
		if err != nil {
			return sui.Argument{}, sui.Argument{}, err
		}
		balances := SwapBalances{BalanceX: input, BalanceY: output}
		limit := new(big.Int).SetUint64(4295048016)
		if !p.XForY {
			balances = SwapBalances{BalanceX: output, BalanceY: input}
			var ok bool
			limit, ok = new(big.Int).SetString("79226673515401279992447579055", 10)
			if !ok {
				return sui.Argument{}, sui.Argument{}, fmt.Errorf("failed to build swap transaction: price_limit=invalid")
			}
		}
		_, err = AppendAtomicSwap(b, deployment, AtomicSwapParams{Pool: p.Pool, Balances: balances, XForY: p.XForY, AmountIn: amount, SqrtPriceLimit: limit})
		return input, output, err
	})
	if err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build momentum swap transaction: %w", err)
	}
	return tx, nil
}
