package sui

import "testing"

// TestCoinCommandsComposeAndCopy verifies mutable arguments cannot alter built snapshots.
//
// Version:
//   - 2026-09-24: Added.
func TestCoinCommandsComposeAndCopy(t *testing.T) {
	builder := NewProgrammableTransactionBuilder()
	amount, err := PureUint64(builder, 10)
	if err != nil {
		t.Fatal(err)
	}
	address, err := builder.Pure(transactionTestAddress(t, "0x1").Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.Pure([]byte{}); err != nil {
		t.Fatal(err)
	}
	split, err := builder.SplitCoins(SplitCoins{Coin: Argument{Kind: ArgumentKindGas}, Amounts: []Argument{amount, amount}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := NestedResult(split, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NestedResult(split, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.MergeCoins(MergeCoins{Destination: first, Sources: []Argument{second}}); err != nil {
		t.Fatal(err)
	}
	if err := builder.TransferObjects(TransferObjects{Objects: []Argument{first}, Address: address}); err != nil {
		t.Fatal(err)
	}
	*first.Subresult = 9
	ptb, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	*ptb.Commands[1].MergeCoins.Destination.Subresult = 8
	ptb.Commands[0].SplitCoins.Amounts[0].Index = 600
	other, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if *other.Commands[1].MergeCoins.Destination.Subresult != 0 || other.Commands[0].SplitCoins.Amounts[0].Index != 0 || other.Inputs[2].Pure == nil {
		t.Fatal("builder snapshot shares state or loses empty BCS")
	}
	rpc, err := transactionToRPC(SimulationRequest{Sender: transactionTestAddress(t, "0x1"), Transaction: other})
	if err != nil {
		t.Fatal(err)
	}
	commands := rpc.GetKind().GetProgrammableTransaction().GetCommands()
	if commands[0].GetSplitCoins() == nil || commands[1].GetMergeCoins() == nil || commands[2].GetTransferObjects() == nil {
		t.Fatal("native commands missing from RPC")
	}
	if commands[0].GetSplitCoins().GetCoin().GetKind().String() != "GAS" {
		t.Fatal("gas argument changed")
	}
}

// TestCoinCommandValidation rejects missing or invalid command arguments.
//
// Version:
//   - 2026-09-24: Added.
func TestCoinCommandValidation(t *testing.T) {
	builder := NewProgrammableTransactionBuilder()
	if _, err := builder.SplitCoins(SplitCoins{}); err == nil {
		t.Fatal("accepted empty split")
	}
	if err := builder.MergeCoins(MergeCoins{}); err == nil {
		t.Fatal("accepted empty merge")
	}
	if err := builder.TransferObjects(TransferObjects{}); err == nil {
		t.Fatal("accepted empty transfer")
	}
	if _, err := (*ProgrammableTransactionBuilder)(nil).SplitCoins(SplitCoins{}); err == nil {
		t.Fatal("accepted nil builder")
	}
	builder.transaction.Commands = make([]Command, maxTransactionElements)
	if _, err := builder.addCommand(Command{}); err == nil {
		t.Fatal("command index overflow")
	}
}

// TestCoinBalanceHelpersCompose verifies the framework calls required by swap funding.
//
// Version:
//   - 2026-09-24: Added.
func TestCoinBalanceHelpersCompose(t *testing.T) {
	builder := NewProgrammableTransactionBuilder()
	amount, err := PureUint64(builder, 10)
	if err != nil {
		t.Fatal(err)
	}
	split, err := builder.SplitCoins(SplitCoins{Coin: Argument{Kind: ArgumentKindGas}, Amounts: []Argument{amount}})
	if err != nil {
		t.Fatal(err)
	}
	coin, err := NestedResult(split, 0)
	if err != nil {
		t.Fatal(err)
	}
	balance, err := AppendCoinIntoBalance(builder, "0x2::sui::SUI", coin)
	if err != nil {
		t.Fatal(err)
	}
	if err := AppendDestroyZeroBalance(builder, "0x2::sui::SUI", balance); err != nil {
		t.Fatal(err)
	}
	ptb, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if ptb.Commands[1].MoveCall.Function != "into_balance" || ptb.Commands[2].MoveCall.Function != "destroy_zero" {
		t.Fatal("unexpected framework calls")
	}
}
