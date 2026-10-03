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
	XForY                      bool
	AmountIn, MinimumAmountOut uint64
	// AmountOut and MaximumAmountIn select exact output; input-mode amounts must be zero.
	AmountOut, MaximumAmountIn           uint64
	InputCoins, GasCoins                 []sui.Coin
	GasPrice, GasBudget, ExpirationEpoch uint64
}

// BuildSwapTransaction builds a funded swap with enforced input and output limits.
// AmountIn and MinimumAmountOut select exact input with full input consumption.
// AmountOut and MaximumAmountIn select exact output with full output enforcement.
// Unused input remains with or is refunded to Sender, independently of Recipient.
//
// Version:
//   - 2026-10-02: Support capped exact-output swaps and sender refunds.
//   - 2026-09-28: Added.
func BuildSwapTransaction(deployment Deployment, p SwapTransactionParams) (sui.TransactionData, error) {
	if err := deployment.Validate(); err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build momentum swap transaction: %w", err)
	}
	typeIn, typeOut := p.Pool.CoinTypeX, p.Pool.CoinTypeY
	if !p.XForY {
		typeIn, typeOut = typeOut, typeIn
	}
	tx, err := suiswap.Build(suiswap.Params{Sender: p.Sender, Recipient: p.Recipient, CoinTypeIn: typeIn, CoinTypeOut: typeOut, AmountIn: p.AmountIn, MinimumAmountOut: p.MinimumAmountOut, AmountOut: p.AmountOut, MaximumAmountIn: p.MaximumAmountIn, InputCoins: p.InputCoins, GasCoins: p.GasCoins, GasPrice: p.GasPrice, GasBudget: p.GasBudget, ExpirationEpoch: p.ExpirationEpoch}, func(b *sui.ProgrammableTransactionBuilder, coin, amount sui.Argument) (sui.Argument, sui.Argument, error) {
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
		_, err = appendAtomicSwap(b, deployment, p.Pool, balances, p.XForY, p.AmountOut == 0, amount, limit)
		return input, output, err
	})
	if err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build momentum swap transaction: %w", err)
	}
	return tx, nil
}
