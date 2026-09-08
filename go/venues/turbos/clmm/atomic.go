package clmm

import (
	"fmt"
	"math/big"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type FlashSwapParams struct {
	Pool                   Pool
	Recipient              onchainSui.Address
	A2B                    bool
	Amount                 *big.Int
	AmountSpecifiedIsInput bool
	SqrtPriceLimit         *big.Int
}

type FlashSwapArguments struct {
	Pool      onchainSui.Argument
	CoinA     onchainSui.Argument
	CoinB     onchainSui.Argument
	Receipt   onchainSui.Argument
	Versioned onchainSui.Argument
}

type AtomicSwapParams struct {
	Pool           Pool
	InputCoin      onchainSui.Argument
	AmountIn       onchainSui.Argument
	MinimumOut     uint64
	SqrtPriceLimit *big.Int
	A2B            bool
	Recipient      onchainSui.Address
	DeadlineMS     uint64
}

// AppendAtomicSwap appends an exact-input Turbos swap whose amount is produced by an earlier PTB command.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Turbos deployment.
//   - params: Atomic swap parameters.
//
// Returns:
//   - Returned coin arguments ordered by pool coin type.
//   - Validation or build error.
//
// Version:
//   - 2026-09-01: Added.
func AppendAtomicSwap(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, params AtomicSwapParams) (SwapArguments, error) {
	if builder == nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: %w", err)
	}
	if params.Pool.Address.IsZero() || params.Pool.InitialVersion == 0 || !validU128(params.SqrtPriceLimit) || params.Recipient.IsZero() || params.DeadlineMS == 0 {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: parameters=invalid")
	}
	inputType := params.Pool.CoinTypeA
	function := "swap_a_b_with_return_"
	if !params.A2B {
		inputType = params.Pool.CoinTypeB
		function = "swap_b_a_with_return_"
	}
	coinVector, err := builder.MakeMoveVec(onchainSui.MakeMoveVec{ElementType: "0x2::coin::Coin<" + inputType + ">", Elements: []onchainSui.Argument{params.InputCoin}})
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: %w", err)
	}
	pool, err := builder.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: params.Pool.Address, Version: params.Pool.InitialVersion, Mutable: true})
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: %w", err)
	}
	minimum, _ := builder.Pure(bcsUint64(params.MinimumOut))
	limit, _ := builder.Pure(bcsUint128(params.SqrtPriceLimit))
	exact, _ := builder.Pure(bcsBool(true))
	recipient, _ := builder.Pure(bcsAddress(params.Recipient))
	deadline, _ := builder.Pure(bcsUint64(params.DeadlineMS))
	clock := deployment.Clock
	clock.Mutable = false
	clockArg, err := builder.Object(onchainSui.InputKindShared, clock)
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: %w", err)
	}
	versioned := deployment.Versioned
	versioned.Mutable = false
	versionedArg, err := builder.Object(onchainSui.InputKindShared, versioned)
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: %w", err)
	}
	result, err := builder.MoveCall(onchainSui.MoveCall{Package: deployment.PublishedAt, Module: deployment.RouterModule, Function: function, TypeArguments: []string{params.Pool.CoinTypeA, params.Pool.CoinTypeB, params.Pool.FeeType}, Arguments: []onchainSui.Argument{pool, coinVector, params.AmountIn, minimum, limit, exact, recipient, deadline, clockArg, versionedArg}})
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: %w", err)
	}
	first, err := onchainSui.NestedResult(result, 0)
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: %w", err)
	}
	second, err := onchainSui.NestedResult(result, 1)
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm atomic swap: %w", err)
	}
	if params.A2B {
		return SwapArguments{CoinA: second, CoinB: first}, nil
	}
	return SwapArguments{CoinA: first, CoinB: second}, nil
}

// AppendFlashSwap appends a Turbos flash swap to a programmable transaction.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Turbos deployment.
//   - params: Flash-swap parameters.
//
// Returns:
//   - Arguments required by the route and repayment command.
//   - Validation or build error.
//
// Version:
//   - 2026-09-01: Added.
func AppendFlashSwap(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, params FlashSwapParams) (FlashSwapArguments, error) {
	if builder == nil {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: %w", err)
	}
	if params.Pool.Address.IsZero() || params.Pool.InitialVersion == 0 || params.Recipient.IsZero() || !validU128(params.Amount) || !validU128(params.SqrtPriceLimit) {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: parameters=invalid")
	}

	pool, err := builder.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: params.Pool.Address, Version: params.Pool.InitialVersion, Mutable: true})
	if err != nil {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: %w", err)
	}
	recipient, _ := builder.Pure(bcsAddress(params.Recipient))
	a2b, _ := builder.Pure(bcsBool(params.A2B))
	amount, _ := builder.Pure(bcsUint128(params.Amount))
	exactInput, _ := builder.Pure(bcsBool(params.AmountSpecifiedIsInput))
	limit, _ := builder.Pure(bcsUint128(params.SqrtPriceLimit))
	clock := deployment.Clock
	clock.Mutable = false
	clockArg, err := builder.Object(onchainSui.InputKindShared, clock)
	if err != nil {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: %w", err)
	}
	versioned := deployment.Versioned
	versioned.Mutable = false
	versionedArg, err := builder.Object(onchainSui.InputKindShared, versioned)
	if err != nil {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: %w", err)
	}
	result, err := builder.MoveCall(onchainSui.MoveCall{
		Package:       deployment.PublishedAt,
		Module:        deployment.PoolModule,
		Function:      "flash_swap",
		TypeArguments: []string{params.Pool.CoinTypeA, params.Pool.CoinTypeB, params.Pool.FeeType},
		Arguments:     []onchainSui.Argument{pool, recipient, a2b, amount, exactInput, limit, clockArg, versionedArg},
	})
	if err != nil {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: %w", err)
	}
	coinA, err := onchainSui.NestedResult(result, 0)
	if err != nil {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: %w", err)
	}
	coinB, err := onchainSui.NestedResult(result, 1)
	if err != nil {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: %w", err)
	}
	receipt, err := onchainSui.NestedResult(result, 2)
	if err != nil {
		return FlashSwapArguments{}, fmt.Errorf("failed to append turbos clmm flash swap: %w", err)
	}
	return FlashSwapArguments{Pool: pool, CoinA: coinA, CoinB: coinB, Receipt: receipt, Versioned: versionedArg}, nil
}

// AppendRepayFlashSwap appends repayment of a Turbos flash swap.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Turbos deployment.
//   - pool: Pool type metadata.
//   - flash: Arguments returned by AppendFlashSwap, with repayment coins supplied by the completed route.
//
// Returns:
//   - Validation or build error.
//
// Version:
//   - 2026-09-01: Added.
func AppendRepayFlashSwap(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, pool Pool, flash FlashSwapArguments) error {
	if builder == nil {
		return fmt.Errorf("failed to append turbos clmm flash swap repayment: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return fmt.Errorf("failed to append turbos clmm flash swap repayment: %w", err)
	}
	if pool.Address.IsZero() || pool.InitialVersion == 0 {
		return fmt.Errorf("failed to append turbos clmm flash swap repayment: pool=invalid")
	}
	_, err := builder.MoveCall(onchainSui.MoveCall{
		Package:       deployment.PublishedAt,
		Module:        deployment.PoolModule,
		Function:      "repay_flash_swap",
		TypeArguments: []string{pool.CoinTypeA, pool.CoinTypeB, pool.FeeType},
		Arguments:     []onchainSui.Argument{flash.Pool, flash.CoinA, flash.CoinB, flash.Receipt, flash.Versioned},
	})
	if err != nil {
		return fmt.Errorf("failed to append turbos clmm flash swap repayment: %w", err)
	}
	return nil
}
