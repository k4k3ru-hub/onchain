package sui

import "testing"

func TestBalanceHelpersBuildConsumableProfitFlow(t *testing.T) {
	builder := NewProgrammableTransactionBuilder()
	balance, err := AppendZeroBalance(builder, "0x2::sui::SUI")
	if err != nil {
		t.Fatalf("AppendZeroBalance() returned an unexpected error: %v", err)
	}
	amount, err := PureUint64(builder, 100)
	if err != nil {
		t.Fatalf("PureUint64() returned an unexpected error: %v", err)
	}
	split, err := AppendBalanceSplit(builder, "0x2::sui::SUI", balance, amount)
	if err != nil {
		t.Fatalf("AppendBalanceSplit() returned an unexpected error: %v", err)
	}
	if err := AppendBalanceJoin(builder, "0x2::sui::SUI", balance, split); err != nil {
		t.Fatalf("AppendBalanceJoin() returned an unexpected error: %v", err)
	}
	if _, err := AppendBalanceValue(builder, "0x2::sui::SUI", balance); err != nil {
		t.Fatalf("AppendBalanceValue() returned an unexpected error: %v", err)
	}
	recipient, _ := ParseAddress("0x7")
	if err := AppendTransferBalance(builder, "0x2::sui::SUI", balance, recipient); err != nil {
		t.Fatalf("AppendTransferBalance() returned an unexpected error: %v", err)
	}
	transaction, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned an unexpected error: %v", err)
	}
	wantFunctions := []string{"zero", "split", "join", "value", "from_balance", "public_transfer"}
	if len(transaction.Commands) != len(wantFunctions) {
		t.Fatalf("command count = %d, want %d", len(transaction.Commands), len(wantFunctions))
	}
	for index, want := range wantFunctions {
		if transaction.Commands[index].MoveCall == nil || transaction.Commands[index].MoveCall.Function != want {
			t.Fatalf("command[%d] = %+v, want function %q", index, transaction.Commands[index], want)
		}
	}
}
