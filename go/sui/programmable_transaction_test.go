package sui

import (
	"testing"

	"github.com/mr-tron/base58"
)

func TestProgrammableTransactionBuilderBuildsMoveCall(t *testing.T) {
	packageAddress, _ := ParseAddress("0x1")
	objectAddress, _ := ParseAddress("0x2")
	digestBytes := make([]byte, 32)
	digestBytes[0] = 1
	digest, _ := ParseObjectDigest(base58.Encode(digestBytes))
	builder := NewProgrammableTransactionBuilder()
	object, err := builder.Object(InputKindImmutableOrOwned, ObjectInput{Address: objectAddress, Version: 7, Digest: digest})
	if err != nil {
		t.Fatalf("Object() returned an unexpected error: %v", err)
	}
	amount, err := builder.Pure([]byte{1, 0, 0, 0, 0, 0, 0, 0})
	if err != nil {
		t.Fatalf("Pure() returned an unexpected error: %v", err)
	}
	result, err := builder.MoveCall(MoveCall{Package: packageAddress, Module: "pool", Function: "quote", TypeArguments: []string{"0x2::sui::SUI"}, Arguments: []Argument{object, amount}})
	if err != nil {
		t.Fatalf("MoveCall() returned an unexpected error: %v", err)
	}
	nested, err := NestedResult(result, 0)
	if err != nil || nested.Subresult == nil || *nested.Subresult != 0 {
		t.Fatalf("NestedResult() = %+v, %v", nested, err)
	}
	transaction, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned an unexpected error: %v", err)
	}
	if len(transaction.Inputs) != 2 || len(transaction.Commands) != 1 || transaction.Commands[0].Function != "quote" {
		t.Fatalf("Build() = %+v", transaction)
	}
	transaction.Inputs[1].Pure[0] = 9
	second, _ := builder.Build()
	if second.Inputs[1].Pure[0] != 1 {
		t.Fatal("Build() returned mutable builder-owned input")
	}
}

func TestProgrammableTransactionRejectsForwardResult(t *testing.T) {
	packageAddress, _ := ParseAddress("0x1")
	builder := NewProgrammableTransactionBuilder()
	_, err := builder.MoveCall(MoveCall{Package: packageAddress, Module: "pool", Function: "quote", Arguments: []Argument{{Kind: ArgumentKindResult, Index: 0}}})
	if err == nil {
		t.Fatal("MoveCall() error = nil, want invalid result error")
	}
}
