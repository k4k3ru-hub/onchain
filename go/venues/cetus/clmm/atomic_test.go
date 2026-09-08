package clmm

import (
	"math/big"
	"testing"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

// TestAppendFlashLoanAndRepayment verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestAppendFlashLoanAndRepayment(t *testing.T) {
	poolAddress, _ := onchainSui.ParseAddress("0x9")
	pool := Pool{Address: poolAddress, InitialVersion: 3, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC"}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	loan, err := AppendFlashLoan(builder, testDeployment(), pool, true, 100)
	if err != nil {
		t.Fatalf("AppendFlashLoan() returned an unexpected error: %v", err)
	}
	if err := AppendRepayFlashLoan(builder, testDeployment(), pool, loan, loan.BalanceA, loan.BalanceB); err != nil {
		t.Fatalf("AppendRepayFlashLoan() returned an unexpected error: %v", err)
	}
	transaction, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned an unexpected error: %v", err)
	}
	if len(transaction.Commands) != 2 || transaction.Commands[0].MoveCall == nil || transaction.Commands[0].MoveCall.Function != "flash_loan" || transaction.Commands[1].MoveCall == nil || transaction.Commands[1].MoveCall.Function != "repay_flash_loan" {
		t.Fatalf("transaction commands = %+v", transaction.Commands)
	}
	if !transaction.Inputs[1].Object.Mutable {
		t.Fatalf("pool input is not mutable: %+v", transaction.Inputs[1])
	}
}

// TestAppendFlashSwapAndBalanceSwap verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestAppendFlashSwapAndBalanceSwap(t *testing.T) {
	poolAddress, _ := onchainSui.ParseAddress("0x9")
	pool := Pool{Address: poolAddress, InitialVersion: 3, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC"}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	flashSwap, err := AppendFlashSwap(builder, testDeployment(), pool, true, true, 100, big.NewInt(5))
	if err != nil {
		t.Fatalf("AppendFlashSwap() returned an unexpected error: %v", err)
	}
	if _, err := AppendFlashSwapPayAmount(builder, testDeployment(), pool, flashSwap); err != nil {
		t.Fatalf("AppendFlashSwapPayAmount() returned an unexpected error: %v", err)
	}
	amount, err := onchainSui.AppendBalanceValue(builder, pool.CoinTypeB, flashSwap.BalanceB)
	if err != nil {
		t.Fatalf("AppendBalanceValue() returned an unexpected error: %v", err)
	}
	if _, err := AppendSwap(builder, testDeployment(), pool, SwapBalances{BalanceA: flashSwap.BalanceA, BalanceB: flashSwap.BalanceB}, false, true, amount, big.NewInt(5)); err != nil {
		t.Fatalf("AppendSwap() returned an unexpected error: %v", err)
	}
	transaction, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned an unexpected error: %v", err)
	}
	functions := []string{"flash_swap", "swap_pay_amount", "value", "flash_swap", "swap_pay_amount", "zero", "split", "join", "join", "repay_flash_swap"}
	if len(transaction.Commands) != len(functions) {
		t.Fatalf("command count = %d, want %d", len(transaction.Commands), len(functions))
	}
	for index, function := range functions {
		if transaction.Commands[index].MoveCall == nil || transaction.Commands[index].MoveCall.Function != function {
			t.Fatalf("command[%d] = %+v, want function %q", index, transaction.Commands[index], function)
		}
	}
}
