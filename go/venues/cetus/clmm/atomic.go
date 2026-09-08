package clmm

import (
	"fmt"
	"math/big"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type FlashSwap struct {
	Config   onchainSui.Argument
	Pool     onchainSui.Argument
	BalanceA onchainSui.Argument
	BalanceB onchainSui.Argument
	Receipt  onchainSui.Argument
}

type SwapBalances struct {
	BalanceA onchainSui.Argument
	BalanceB onchainSui.Argument
}

// AppendFlashSwap appends a Cetus CLMM flash swap to a programmable transaction.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Cetus deployment.
//   - pool: Cetus pool state.
//   - a2b: Swap direction.
//   - byAmountIn: Whether amount is exact input.
//   - amount: Swap amount in atomic units.
//   - sqrtPriceLimit: Q64.64 price limit.
//
// Returns:
//   - Flash-swap transaction arguments.
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func AppendFlashSwap(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, pool Pool, a2b, byAmountIn bool, amount uint64, sqrtPriceLimit *big.Int) (FlashSwap, error) {
	amountArgument, err := onchainSui.PureUint64(builder, amount)
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	return appendFlashSwap(builder, deployment, pool, a2b, byAmountIn, amountArgument, sqrtPriceLimit)
}

// AppendFlashSwapPayAmount appends a call that reads the exact flash-swap repayment amount.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Cetus deployment.
//   - pool: Cetus pool state.
//   - flashSwap: Original flash-swap arguments.
//
// Returns:
//   - Repayment u64 result argument.
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func AppendFlashSwapPayAmount(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, pool Pool, flashSwap FlashSwap) (onchainSui.Argument, error) {
	if builder == nil {
		return onchainSui.Argument{}, fmt.Errorf("failed to append cetus clmm flash swap pay amount: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return onchainSui.Argument{}, fmt.Errorf("failed to append cetus clmm flash swap pay amount: %w", err)
	}
	if pool.CoinTypeA == "" || pool.CoinTypeB == "" {
		return onchainSui.Argument{}, fmt.Errorf("failed to append cetus clmm flash swap pay amount: pool=invalid")
	}
	result, err := builder.MoveCall(onchainSui.MoveCall{Package: deployment.PublishedAt, Module: deployment.PoolModule, Function: "swap_pay_amount", TypeArguments: []string{pool.CoinTypeA, pool.CoinTypeB}, Arguments: []onchainSui.Argument{flashSwap.Receipt}})
	if err != nil {
		return onchainSui.Argument{}, fmt.Errorf("failed to append cetus clmm flash swap pay amount: %w", err)
	}
	return result, nil
}

// AppendRepayFlashSwap appends repayment for a previously created Cetus flash swap.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Cetus deployment.
//   - pool: Cetus pool state.
//   - flashSwap: Original flash-swap arguments.
//   - balanceA: Exact repayment balance A.
//   - balanceB: Exact repayment balance B.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
func AppendRepayFlashSwap(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, pool Pool, flashSwap FlashSwap, balanceA, balanceB onchainSui.Argument) error {
	if builder == nil {
		return fmt.Errorf("failed to append cetus clmm flash swap repayment: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return fmt.Errorf("failed to append cetus clmm flash swap repayment: %w", err)
	}
	if pool.CoinTypeA == "" || pool.CoinTypeB == "" {
		return fmt.Errorf("failed to append cetus clmm flash swap repayment: pool=invalid")
	}
	_, err := builder.MoveCall(onchainSui.MoveCall{Package: deployment.PublishedAt, Module: deployment.PoolModule, Function: "repay_flash_swap", TypeArguments: []string{pool.CoinTypeA, pool.CoinTypeB}, Arguments: []onchainSui.Argument{flashSwap.Config, flashSwap.Pool, balanceA, balanceB, flashSwap.Receipt}})
	if err != nil {
		return fmt.Errorf("failed to append cetus clmm flash swap repayment: %w", err)
	}
	return nil
}

// AppendSwap appends a Cetus CLMM swap using transaction-owned balances.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Cetus deployment.
//   - pool: Cetus pool state.
//   - balances: Coin A and coin B balances.
//   - a2b: Swap direction.
//   - byAmountIn: Whether amount is exact input.
//   - amount: Runtime u64 amount argument.
//   - sqrtPriceLimit: Q64.64 price limit.
//
// Returns:
//   - Mutated transaction-owned balances.
//   - Validation error.
//
// Version:
//   - 2026-08-31: Added.
//   - 2026-09-01: Consumed the zero-valued flash-swap balance before repayment.
func AppendSwap(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, pool Pool, balances SwapBalances, a2b, byAmountIn bool, amount onchainSui.Argument, sqrtPriceLimit *big.Int) (SwapBalances, error) {
	flashSwap, err := appendFlashSwap(builder, deployment, pool, a2b, byAmountIn, amount, sqrtPriceLimit)
	if err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
	}
	payAmount, err := AppendFlashSwapPayAmount(builder, deployment, pool, flashSwap)
	if err != nil {
		return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
	}
	if a2b {
		payA, err := onchainSui.AppendBalanceSplit(builder, pool.CoinTypeA, balances.BalanceA, payAmount)
		if err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
		payB, err := onchainSui.AppendZeroBalance(builder, pool.CoinTypeB)
		if err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
		if err := onchainSui.AppendBalanceJoin(builder, pool.CoinTypeB, balances.BalanceB, flashSwap.BalanceB); err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
		if err := onchainSui.AppendBalanceJoin(builder, pool.CoinTypeA, balances.BalanceA, flashSwap.BalanceA); err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
		if err := AppendRepayFlashSwap(builder, deployment, pool, flashSwap, payA, payB); err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
	} else {
		payA, err := onchainSui.AppendZeroBalance(builder, pool.CoinTypeA)
		if err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
		payB, err := onchainSui.AppendBalanceSplit(builder, pool.CoinTypeB, balances.BalanceB, payAmount)
		if err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
		if err := onchainSui.AppendBalanceJoin(builder, pool.CoinTypeA, balances.BalanceA, flashSwap.BalanceA); err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
		if err := onchainSui.AppendBalanceJoin(builder, pool.CoinTypeB, balances.BalanceB, flashSwap.BalanceB); err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
		if err := AppendRepayFlashSwap(builder, deployment, pool, flashSwap, payA, payB); err != nil {
			return SwapBalances{}, fmt.Errorf("failed to append cetus clmm swap: %w", err)
		}
	}
	return balances, nil
}

func appendFlashSwap(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, pool Pool, a2b, byAmountIn bool, amount onchainSui.Argument, sqrtPriceLimit *big.Int) (FlashSwap, error) {
	if builder == nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	if pool.Address.IsZero() || pool.InitialVersion == 0 || pool.CoinTypeA == "" || pool.CoinTypeB == "" {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: pool=invalid")
	}
	if sqrtPriceLimit == nil || sqrtPriceLimit.Sign() <= 0 || sqrtPriceLimit.BitLen() > 128 {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: sqrt_price_limit=invalid")
	}
	config := deployment.GlobalConfig
	config.Mutable = false
	configArgument, err := builder.Object(onchainSui.InputKindShared, config)
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	poolArgument, err := builder.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: pool.Address, Version: pool.InitialVersion, Mutable: true})
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	direction, err := builder.Pure(bcsBool(a2b))
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	exact, err := builder.Pure(bcsBool(byAmountIn))
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	priceLimit, err := builder.Pure(bcsUint128(sqrtPriceLimit))
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	clock := deployment.Clock
	clock.Mutable = false
	clockArgument, err := builder.Object(onchainSui.InputKindShared, clock)
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	result, err := builder.MoveCall(onchainSui.MoveCall{Package: deployment.PublishedAt, Module: deployment.PoolModule, Function: "flash_swap", TypeArguments: []string{pool.CoinTypeA, pool.CoinTypeB}, Arguments: []onchainSui.Argument{configArgument, poolArgument, direction, exact, amount, priceLimit, clockArgument}})
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	balanceA, err := onchainSui.NestedResult(result, 0)
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	balanceB, err := onchainSui.NestedResult(result, 1)
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	receipt, err := onchainSui.NestedResult(result, 2)
	if err != nil {
		return FlashSwap{}, fmt.Errorf("failed to append cetus clmm flash swap: %w", err)
	}
	return FlashSwap{Config: configArgument, Pool: poolArgument, BalanceA: balanceA, BalanceB: balanceB, Receipt: receipt}, nil
}

type FlashLoan struct {
	Config   onchainSui.Argument
	Pool     onchainSui.Argument
	BalanceA onchainSui.Argument
	BalanceB onchainSui.Argument
	Receipt  onchainSui.Argument
}

// AppendFlashLoan appends a Cetus flash-loan Move call to a programmable transaction.
//
// The returned balances and receipt are transaction arguments that callers can
// route through other DEX calls before appending repayment.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Cetus deployment.
//   - pool: Cetus pool state.
//   - loanA: Whether to borrow coin A instead of coin B.
//   - amount: Borrow amount in atomic units.
//
// Returns:
//   - Flash-loan transaction arguments.
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func AppendFlashLoan(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, pool Pool, loanA bool, amount uint64) (FlashLoan, error) {
	if builder == nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: %w", err)
	}
	if pool.Address.IsZero() || pool.InitialVersion == 0 || pool.CoinTypeA == "" || pool.CoinTypeB == "" {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: pool=invalid")
	}
	if amount == 0 {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: amount=empty")
	}
	configObject := deployment.GlobalConfig
	configObject.Mutable = false
	config, err := builder.Object(onchainSui.InputKindShared, configObject)
	if err != nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: %w", err)
	}
	poolArgument, err := builder.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: pool.Address, Version: pool.InitialVersion, Mutable: true})
	if err != nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: %w", err)
	}
	loanAArgument, err := builder.Pure(bcsBool(loanA))
	if err != nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: %w", err)
	}
	amountArgument, err := builder.Pure(bcsUint64(amount))
	if err != nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: %w", err)
	}
	result, err := builder.MoveCall(onchainSui.MoveCall{
		Package:       deployment.PublishedAt,
		Module:        deployment.PoolModule,
		Function:      "flash_loan",
		TypeArguments: []string{pool.CoinTypeA, pool.CoinTypeB},
		Arguments:     []onchainSui.Argument{config, poolArgument, loanAArgument, amountArgument},
	})
	if err != nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: %w", err)
	}
	balanceA, err := onchainSui.NestedResult(result, 0)
	if err != nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: %w", err)
	}
	balanceB, err := onchainSui.NestedResult(result, 1)
	if err != nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: %w", err)
	}
	receipt, err := onchainSui.NestedResult(result, 2)
	if err != nil {
		return FlashLoan{}, fmt.Errorf("failed to append cetus clmm flash loan: %w", err)
	}
	return FlashLoan{Config: config, Pool: poolArgument, BalanceA: balanceA, BalanceB: balanceB, Receipt: receipt}, nil
}

// AppendRepayFlashLoan appends repayment for a previously created Cetus flash loan.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Cetus deployment.
//   - pool: Cetus pool state.
//   - loan: Original flash-loan arguments.
//   - balanceA: Coin A balance after arbitrage routing.
//   - balanceB: Coin B balance after arbitrage routing.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func AppendRepayFlashLoan(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, pool Pool, loan FlashLoan, balanceA, balanceB onchainSui.Argument) error {
	if builder == nil {
		return fmt.Errorf("failed to append cetus clmm flash loan repayment: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return fmt.Errorf("failed to append cetus clmm flash loan repayment: %w", err)
	}
	if pool.CoinTypeA == "" || pool.CoinTypeB == "" {
		return fmt.Errorf("failed to append cetus clmm flash loan repayment: pool=invalid")
	}
	_, err := builder.MoveCall(onchainSui.MoveCall{
		Package:       deployment.PublishedAt,
		Module:        deployment.PoolModule,
		Function:      "repay_flash_loan",
		TypeArguments: []string{pool.CoinTypeA, pool.CoinTypeB},
		Arguments:     []onchainSui.Argument{loan.Config, loan.Pool, balanceA, balanceB, loan.Receipt},
	})
	if err != nil {
		return fmt.Errorf("failed to append cetus clmm flash loan repayment: %w", err)
	}
	return nil
}
