package sui

import (
	"strings"
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
	if len(transaction.Inputs) != 2 || len(transaction.Commands) != 1 || transaction.Commands[0].MoveCall == nil || transaction.Commands[0].MoveCall.Function != "quote" {
		t.Fatalf("Build() = %+v", transaction)
	}
	transaction.Inputs[1].Pure[0] = 9
	second, _ := builder.Build()
	if second.Inputs[1].Pure[0] != 1 {
		t.Fatal("Build() returned mutable builder-owned input")
	}
}

func TestProgrammableTransactionBuilderReusesSharedObjectInput(t *testing.T) {
	builder := NewProgrammableTransactionBuilder()
	address, _ := ParseAddress("0x6")
	immutable, err := builder.Object(InputKindShared, ObjectInput{Address: address, Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	mutable, err := builder.Object(InputKindShared, ObjectInput{Address: address, Version: 1, Mutable: true})
	if err != nil {
		t.Fatal(err)
	}
	if immutable != mutable {
		t.Fatalf("shared object arguments = %+v and %+v", immutable, mutable)
	}
	builder.transaction.Commands = append(builder.transaction.Commands, Command{Kind: CommandKindMakeMoveVec, MakeMoveVec: &MakeMoveVec{ElementType: "0x2::object::ID", Elements: []Argument{immutable}}})
	transaction, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(transaction.Inputs) != 1 || !transaction.Inputs[0].Object.Mutable {
		t.Fatalf("inputs = %+v", transaction.Inputs)
	}
}

func TestProgrammableTransactionBuilderRejectsConflictingSharedObjectVersion(t *testing.T) {
	builder := NewProgrammableTransactionBuilder()
	address, _ := ParseAddress("0x6")
	if _, err := builder.Object(InputKindShared, ObjectInput{Address: address, Version: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := builder.Object(InputKindShared, ObjectInput{Address: address, Version: 2}); err == nil || !strings.Contains(err.Error(), "shared_object_version=invalid") {
		t.Fatalf("Object() error = %v", err)
	}
}

func TestProgrammableTransactionRejectsDuplicateObjectInputs(t *testing.T) {
	address, _ := ParseAddress("0x6")
	transaction := ProgrammableTransaction{
		Inputs: []ProgrammableTransactionInput{
			{Kind: InputKindShared, Object: ObjectInput{Address: address, Version: 1}},
			{Kind: InputKindShared, Object: ObjectInput{Address: address, Version: 1}},
		},
		Commands: []Command{{Kind: CommandKindMakeMoveVec, MakeMoveVec: &MakeMoveVec{ElementType: "0x2::object::ID", Elements: []Argument{{Kind: ArgumentKindInput, Index: 0}}}}},
	}
	if err := transaction.Validate(); err == nil || !strings.Contains(err.Error(), "object_input=duplicate") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProgrammableTransactionBuilderBuildsMoveVector(t *testing.T) {
	packageAddress, _ := ParseAddress("0x1")
	builder := NewProgrammableTransactionBuilder()
	coin, err := builder.MoveCall(MoveCall{Package: packageAddress, Module: "coin", Function: "zero", TypeArguments: []string{"0x2::sui::SUI"}})
	if err != nil {
		t.Fatalf("MoveCall() returned an unexpected error: %v", err)
	}
	vector, err := builder.MakeMoveVec(MakeMoveVec{ElementType: "0x2::coin::Coin<0x2::sui::SUI>", Elements: []Argument{coin}})
	if err != nil {
		t.Fatalf("MakeMoveVec() returned an unexpected error: %v", err)
	}
	_, err = builder.MoveCall(MoveCall{Package: packageAddress, Module: "router", Function: "swap", Arguments: []Argument{vector}})
	if err != nil {
		t.Fatalf("MoveCall() returned an unexpected error: %v", err)
	}
	transaction, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned an unexpected error: %v", err)
	}
	if len(transaction.Commands) != 3 || transaction.Commands[1].Kind != CommandKindMakeMoveVec || transaction.Commands[1].MakeMoveVec == nil {
		t.Fatalf("Build() = %+v", transaction)
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
