package ammtransaction

import (
	"testing"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

func TestDeriveSwapUsesVaultBalanceDeltas(t *testing.T) {
	vaultA, mintA, vaultB, mintB := address(1), address(2), address(3), address(4)
	transaction := &onchainSolana.Transaction{
		AccountKeys: []onchainSolana.Address{address(9), vaultA, vaultB},
		PreTokenBalances: []onchainSolana.TokenBalance{
			{AccountIndex: 1, Mint: mintA, Amount: "1000"},
			{AccountIndex: 2, Mint: mintB, Amount: "2000"},
		},
		PostTokenBalances: []onchainSolana.TokenBalance{
			{AccountIndex: 1, Mint: mintA, Amount: "1010"},
			{AccountIndex: 2, Mint: mintB, Amount: "1980"},
		},
	}

	got, found, err := DeriveSwap(transaction, vaultA, mintA, vaultB, mintB)
	if err != nil || !found {
		t.Fatalf("DeriveSwap() = %+v, %t, %v", got, found, err)
	}
	if got.InputMint != mintA || got.OutputMint != mintB || got.AmountIn != 10 || got.AmountOut != 20 {
		t.Fatalf("DeriveSwap() = %+v", got)
	}
}

func TestDeriveSwapSkipsUnrelatedTransaction(t *testing.T) {
	_, found, err := DeriveSwap(&onchainSolana.Transaction{AccountKeys: []onchainSolana.Address{address(1)}}, address(2), address(3), address(4), address(5))
	if err != nil || found {
		t.Fatalf("DeriveSwap() found = %t error = %v", found, err)
	}
}

func address(value byte) onchainSolana.Address {
	var result onchainSolana.Address
	result[0] = value
	return result
}
