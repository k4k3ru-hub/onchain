package ammtransaction

import (
	"fmt"
	"math/big"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type Swap struct {
	InputMint  onchainSolana.Address
	OutputMint onchainSolana.Address
	AmountIn   uint64
	AmountOut  uint64
}

// DeriveSwap derives one pool-level swap from transaction vault balance deltas.
//
// Parameters:
//   - transaction: confirmed Solana transaction.
//   - vaultA, mintA: first pool vault and mint.
//   - vaultB, mintB: second pool vault and mint.
//
// Returns:
//   - Derived swap.
//   - Whether the transaction changed both vaults as a swap.
//   - Derivation error.
//
// Version:
//   - 2026-09-01: Added.
func DeriveSwap(transaction *onchainSolana.Transaction, vaultA, mintA, vaultB, mintB onchainSolana.Address) (Swap, bool, error) {
	if transaction == nil {
		return Swap{}, false, fmt.Errorf("failed to derive solana amm swap: transaction=null")
	}
	indexA, foundA := accountIndex(transaction.AccountKeys, vaultA)
	indexB, foundB := accountIndex(transaction.AccountKeys, vaultB)
	if !foundA || !foundB {
		return Swap{}, false, nil
	}
	deltaA, okA, err := tokenBalanceDelta(transaction, indexA, mintA)
	if err != nil {
		return Swap{}, false, err
	}
	deltaB, okB, err := tokenBalanceDelta(transaction, indexB, mintB)
	if err != nil {
		return Swap{}, false, err
	}
	if !okA || !okB || deltaA.Sign() == 0 || deltaB.Sign() == 0 || deltaA.Sign() == deltaB.Sign() {
		return Swap{}, false, nil
	}
	if deltaA.Sign() > 0 {
		amountIn, ok := deltaA.Uint64(), deltaA.IsUint64()
		amountOutValue := new(big.Int).Neg(deltaB)
		amountOut, outputOK := amountOutValue.Uint64(), amountOutValue.IsUint64()
		if !ok || !outputOK {
			return Swap{}, false, fmt.Errorf("failed to derive solana amm swap: amount=out_of_range")
		}
		return Swap{InputMint: mintA, OutputMint: mintB, AmountIn: amountIn, AmountOut: amountOut}, true, nil
	}
	amountInValue := new(big.Int).Set(deltaB)
	amountOutValue := new(big.Int).Neg(deltaA)
	amountIn, inputOK := amountInValue.Uint64(), amountInValue.IsUint64()
	amountOut, outputOK := amountOutValue.Uint64(), amountOutValue.IsUint64()
	if !inputOK || !outputOK {
		return Swap{}, false, fmt.Errorf("failed to derive solana amm swap: amount=out_of_range")
	}
	return Swap{InputMint: mintB, OutputMint: mintA, AmountIn: amountIn, AmountOut: amountOut}, true, nil
}

func accountIndex(keys []onchainSolana.Address, address onchainSolana.Address) (uint16, bool) {
	for index, key := range keys {
		if key == address && index <= int(^uint16(0)) {
			return uint16(index), true
		}
	}
	return 0, false
}

func tokenBalanceDelta(transaction *onchainSolana.Transaction, index uint16, mint onchainSolana.Address) (*big.Int, bool, error) {
	pre, preFound, err := tokenBalance(transaction.PreTokenBalances, index, mint)
	if err != nil {
		return nil, false, err
	}
	post, postFound, err := tokenBalance(transaction.PostTokenBalances, index, mint)
	if err != nil {
		return nil, false, err
	}
	if !preFound || !postFound {
		return nil, false, nil
	}
	return new(big.Int).Sub(post, pre), true, nil
}

func tokenBalance(values []onchainSolana.TokenBalance, index uint16, mint onchainSolana.Address) (*big.Int, bool, error) {
	for _, value := range values {
		if value.AccountIndex != index || value.Mint != mint {
			continue
		}
		amount, ok := new(big.Int).SetString(value.Amount, 10)
		if !ok || amount.Sign() < 0 {
			return nil, false, fmt.Errorf("failed to derive solana amm swap: token_balance=invalid account_index=%d", index)
		}
		return amount, true, nil
	}
	return nil, false, nil
}
