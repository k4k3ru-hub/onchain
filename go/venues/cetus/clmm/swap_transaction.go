package clmm

import (
	"fmt"
	"math"
	"math/big"

	sui "github.com/k4k3ru-hub/onchain/go/sui"
)

type SwapTransactionParams struct {
	Pool             Pool
	Sender           sui.Address
	Recipient        sui.Address
	A2B              bool
	AmountIn         uint64
	MinimumAmountOut uint64
	InputCoins       []sui.Coin
	GasCoins         []sui.Coin
	GasPrice         uint64
	GasBudget        uint64
	ExpirationEpoch  uint64
}

// BuildSwapTransaction builds a funded exact-input transaction with a minimum-output guard.
// Coin metadata must be read from chain immediately before calling this function.
// Native SUI input uses GasCoin; other inputs merge and split caller-selected coins.
// Input change remains with the sender. Partial fills abort via destroy_zero.
//
// Version:
//   - 2026-09-24: Added.
func BuildSwapTransaction(deployment Deployment, p SwapTransactionParams) (sui.TransactionData, error) {
	tx, err := buildSwapTransaction(deployment, p)
	if err != nil {
		return sui.TransactionData{}, fmt.Errorf("failed to build cetus swap transaction: %w", err)
	}
	return tx, nil
}

func buildSwapTransaction(deployment Deployment, p SwapTransactionParams) (sui.TransactionData, error) {
	if err := deployment.Validate(); err != nil {
		return sui.TransactionData{}, err
	}
	if p.Sender.IsZero() || p.Recipient.IsZero() || p.AmountIn == 0 || p.GasPrice == 0 || p.GasBudget == 0 {
		return sui.TransactionData{}, fmt.Errorf("failed to validate cetus swap transaction: parameters=invalid")
	}
	typeIn, typeOut := p.Pool.CoinTypeA, p.Pool.CoinTypeB
	if !p.A2B {
		typeIn, typeOut = typeOut, typeIn
	}
	native, err := sui.NormalizeMoveType("0x2::sui::SUI")
	if err != nil {
		return sui.TransactionData{}, err
	}
	typeIn, err = sui.NormalizeMoveType(typeIn)
	if err != nil {
		return sui.TransactionData{}, err
	}
	typeOut, err = sui.NormalizeMoveType(typeOut)
	if err != nil {
		return sui.TransactionData{}, err
	}
	if typeIn == typeOut {
		return sui.TransactionData{}, fmt.Errorf("failed to validate cetus swap transaction: assets=invalid")
	}
	seen := make(map[sui.Address]bool)
	gasTotal, err := validateSwapCoins(p.GasCoins, p.Sender, native, seen)
	if err != nil {
		return sui.TransactionData{}, err
	}
	if gasTotal < p.GasBudget {
		return sui.TransactionData{}, fmt.Errorf("failed to fund cetus swap transaction: gas_balance=insufficient")
	}
	if typeIn == native {
		if len(p.InputCoins) != 0 {
			return sui.TransactionData{}, fmt.Errorf("failed to fund cetus swap transaction: input_coins=invalid")
		}
		if gasTotal-p.GasBudget < p.AmountIn {
			return sui.TransactionData{}, fmt.Errorf("failed to fund cetus swap transaction: input_balance=insufficient")
		}
	} else {
		total, err := validateSwapCoins(p.InputCoins, p.Sender, typeIn, seen)
		if err != nil {
			return sui.TransactionData{}, err
		}
		if total < p.AmountIn {
			return sui.TransactionData{}, fmt.Errorf("failed to fund cetus swap transaction: input_balance=insufficient")
		}
	}
	b := sui.NewProgrammableTransactionBuilder()
	coin := sui.Argument{Kind: sui.ArgumentKindGas}
	if typeIn != native {
		args := make([]sui.Argument, 0, len(p.InputCoins))
		for _, input := range p.InputCoins {
			r := input.Reference
			a, err := b.Object(sui.InputKindImmutableOrOwned, sui.ObjectInput{Address: r.Address, Version: r.Version, Digest: r.Digest})
			if err != nil {
				return sui.TransactionData{}, err
			}
			args = append(args, a)
		}
		coin = args[0]
		if len(args) > 1 {
			if err := b.MergeCoins(sui.MergeCoins{Destination: coin, Sources: args[1:]}); err != nil {
				return sui.TransactionData{}, err
			}
		}
	}
	amount, err := sui.PureUint64(b, p.AmountIn)
	if err != nil {
		return sui.TransactionData{}, err
	}
	split, err := b.SplitCoins(sui.SplitCoins{Coin: coin, Amounts: []sui.Argument{amount}})
	if err != nil {
		return sui.TransactionData{}, err
	}
	inputCoin, err := sui.NestedResult(split, 0)
	if err != nil {
		return sui.TransactionData{}, err
	}
	input, err := sui.AppendCoinIntoBalance(b, typeIn, inputCoin)
	if err != nil {
		return sui.TransactionData{}, err
	}
	output, err := sui.AppendZeroBalance(b, typeOut)
	if err != nil {
		return sui.TransactionData{}, err
	}
	balances := SwapBalances{BalanceA: input, BalanceB: output}
	if !p.A2B {
		balances = SwapBalances{BalanceA: output, BalanceB: input}
	}
	limitText := "4295048016"
	if !p.A2B {
		limitText = "79226673515401279992447579055"
	}
	priceLimit, ok := new(big.Int).SetString(limitText, 10)
	if !ok {
		return sui.TransactionData{}, fmt.Errorf("failed to build cetus swap transaction: price_limit=invalid")
	}
	if _, err := AppendSwap(b, deployment, p.Pool, balances, p.A2B, true, amount, priceLimit); err != nil {
		return sui.TransactionData{}, err
	}
	if err := sui.AppendDestroyZeroBalance(b, typeIn, input); err != nil {
		return sui.TransactionData{}, err
	}
	minimum, err := sui.PureUint64(b, p.MinimumAmountOut)
	if err != nil {
		return sui.TransactionData{}, err
	}
	guard, err := sui.AppendBalanceSplit(b, typeOut, output, minimum)
	if err != nil {
		return sui.TransactionData{}, err
	}
	if err := sui.AppendBalanceJoin(b, typeOut, output, guard); err != nil {
		return sui.TransactionData{}, err
	}
	if err := sui.AppendTransferBalance(b, typeOut, output, p.Recipient); err != nil {
		return sui.TransactionData{}, err
	}
	ptb, err := b.Build()
	if err != nil {
		return sui.TransactionData{}, err
	}
	gas := sui.GasData{Owner: p.Sender, Price: p.GasPrice, Budget: p.GasBudget}
	for _, coin := range p.GasCoins {
		gas.Payment = append(gas.Payment, coin.Reference)
	}
	tx := sui.TransactionData{Transaction: ptb, Sender: p.Sender, GasData: gas, ExpirationEpoch: &p.ExpirationEpoch}
	if err := tx.Validate(); err != nil {
		return sui.TransactionData{}, err
	}
	return tx, nil
}

func validateSwapCoins(coins []sui.Coin, owner sui.Address, coinType string, seen map[sui.Address]bool) (uint64, error) {
	if len(coins) == 0 || len(coins) > 256 {
		return 0, fmt.Errorf("failed to validate cetus swap coins: coins=out_of_range")
	}
	var total uint64
	for _, coin := range coins {
		if err := coin.Reference.Validate(); err != nil {
			return 0, err
		}
		actual, err := sui.NormalizeMoveType(coin.CoinType)
		if err != nil {
			return 0, err
		}
		if coin.Owner != owner || actual != coinType {
			return 0, fmt.Errorf("failed to validate cetus swap coins: coin=mismatch")
		}
		if seen[coin.Reference.Address] {
			return 0, fmt.Errorf("failed to validate cetus swap coins: object=duplicate")
		}
		seen[coin.Reference.Address] = true
		if math.MaxUint64-total < coin.Balance {
			return 0, fmt.Errorf("failed to validate cetus swap coins: balance=out_of_range")
		}
		total += coin.Balance
	}
	return total, nil
}
