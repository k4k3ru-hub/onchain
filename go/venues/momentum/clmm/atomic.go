package clmm

import (
	"fmt"
	"math/big"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type SwapBalances struct {
	BalanceX onchainSui.Argument
	BalanceY onchainSui.Argument
}

type AtomicSwapParams struct {
	Pool           Pool
	Balances       SwapBalances
	XForY          bool
	AmountIn       onchainSui.Argument
	SqrtPriceLimit *big.Int
}

// AppendAtomicSwap appends an exact-input Momentum swap using balances produced by earlier PTB commands.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Momentum deployment.
//   - params: Atomic swap parameters.
//
// Returns:
//   - Updated pool-token balances.
//   - Validation or build error.
//
// Version:
//   - 2026-09-01: Added.
func AppendAtomicSwap(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, params AtomicSwapParams) (SwapBalances, error) {
	return appendAtomicSwap(builder, deployment, params.Pool, params.Balances, params.XForY, true, params.AmountIn, params.SqrtPriceLimit)
}

func appendAtomicSwap(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, pool Pool, balances SwapBalances, xForY, byAmountIn bool, amount onchainSui.Argument, sqrtPriceLimit *big.Int) (SwapBalances, error) {
	if builder == nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	if pool.Address.IsZero() || pool.InitialVersion == 0 || !validU128(sqrtPriceLimit) {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: parameters=invalid")
	}
	flash, err := appendFlashSwap(builder, deployment, pool, xForY, byAmountIn, amount, sqrtPriceLimit)
	if err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	debts, err := builder.MoveCall(onchainSui.MoveCall{Package: deployment.PublishedAt, Module: deployment.TradeModule, Function: "swap_receipt_debts", Arguments: []onchainSui.Argument{flash.Receipt}})
	if err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	debtX, err := onchainSui.NestedResult(debts, 0)
	if err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	debtY, err := onchainSui.NestedResult(debts, 1)
	if err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	if xForY {
		payX, err := onchainSui.AppendBalanceSplit(builder, pool.CoinTypeX, balances.BalanceX, debtX)
		if err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
		}
		if err := onchainSui.AppendBalanceJoin(builder, pool.CoinTypeX, payX, flash.BalanceX); err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
		}
		payY, err := onchainSui.AppendZeroBalance(builder, pool.CoinTypeY)
		if err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
		}
		if err := AppendRepayFlashSwap(builder, deployment, pool, flash, payX, payY); err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
		}
		if err := onchainSui.AppendBalanceJoin(builder, pool.CoinTypeY, balances.BalanceY, flash.BalanceY); err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
		}
		return balances, nil
	}
	payY, err := onchainSui.AppendBalanceSplit(builder, pool.CoinTypeY, balances.BalanceY, debtY)
	if err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	if err := onchainSui.AppendBalanceJoin(builder, pool.CoinTypeY, payY, flash.BalanceY); err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	payX, err := onchainSui.AppendZeroBalance(builder, pool.CoinTypeX)
	if err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	if err := AppendRepayFlashSwap(builder, deployment, pool, flash, payX, payY); err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	if err := onchainSui.AppendBalanceJoin(builder, pool.CoinTypeX, balances.BalanceX, flash.BalanceX); err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append momentum clmm atomic swap: %w", err)
	}
	return balances, nil
}
