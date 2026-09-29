package spot

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
}

// BuildSwapTransaction builds a funded Bluefin exact-input swap with an onchain output guard.
// Unspent input aborts the transaction; input change remains with the sender.
//
// Version:
//   - 2026-09-28: Added.
func BuildSwapTransaction(deployment Deployment, p SwapTransactionParams) (sui.TransactionData, error) {
	if err := deployment.Validate(); err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build bluefin swap transaction: %w", err)
	}
	typeIn, typeOut := p.Pool.CoinTypeA, p.Pool.CoinTypeB
	if !p.A2B {
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
		balances := SwapBalances{BalanceA: input, BalanceB: output}
		// Bluefin swap requires strict interior bounds; its quote helper permits equality.
		limit := new(big.Int).SetUint64(4295048017)
		if !p.A2B {
			balances = SwapBalances{BalanceA: output, BalanceB: input}
			var ok bool
			limit, ok = new(big.Int).SetString("79226673515401279992447579054", 10)
			if !ok {
				return sui.Argument{}, sui.Argument{}, fmt.Errorf("failed to build bluefin swap transaction: price_limit=invalid")
			}
		}
		remaining, err := AppendSwap(b, deployment, p.Pool, balances, p.A2B, true, amount, p.MinimumAmountOut, limit)
		if err != nil {
			return sui.Argument{}, sui.Argument{}, err
		}
		if p.A2B {
			return remaining.BalanceA, remaining.BalanceB, nil
		}
		return remaining.BalanceB, remaining.BalanceA, nil
	})
	if err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build bluefin swap transaction: %w", err)
	}
	return tx, nil
}
