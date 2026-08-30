package sui

import (
	"context"
	"testing"
)

type testSimulationProvider struct {
	request SimulationRequest
	result  *SimulationResult
}

func (p *testSimulationProvider) simulateTransaction(_ context.Context, request SimulationRequest) (*SimulationResult, error) {
	p.request = request
	return p.result, nil
}

func TestSimulateTransactionDelegatesValidatedRequest(t *testing.T) {
	sender, _ := ParseAddress("0x1")
	packageAddress, _ := ParseAddress("0x2")
	builder := NewProgrammableTransactionBuilder()
	_, _ = builder.MoveCall(MoveCall{Package: packageAddress, Module: "fetcher", Function: "quote"})
	transaction, _ := builder.Build()
	provider := &testSimulationProvider{result: &SimulationResult{SuggestedGasPrice: 100}}
	client := &GRPCClient{simulationProvider: provider}
	result, err := client.SimulateTransaction(nil, SimulationRequest{Sender: sender, Transaction: transaction})
	if err != nil {
		t.Fatalf("SimulateTransaction() returned an unexpected error: %v", err)
	}
	if result.SuggestedGasPrice != 100 || provider.request.Sender != sender {
		t.Fatalf("SimulateTransaction() = %+v request=%+v", result, provider.request)
	}
}

func TestTransactionToRPCConvertsMoveVector(t *testing.T) {
	sender, _ := ParseAddress("0x1")
	packageAddress, _ := ParseAddress("0x2")
	builder := NewProgrammableTransactionBuilder()
	coin, err := builder.MoveCall(MoveCall{Package: packageAddress, Module: "coin", Function: "zero", TypeArguments: []string{"0x2::sui::SUI"}})
	if err != nil {
		t.Fatalf("MoveCall() returned an unexpected error: %v", err)
	}
	_, err = builder.MakeMoveVec(MakeMoveVec{ElementType: "0x2::coin::Coin<0x2::sui::SUI>", Elements: []Argument{coin}})
	if err != nil {
		t.Fatalf("MakeMoveVec() returned an unexpected error: %v", err)
	}
	transaction, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned an unexpected error: %v", err)
	}
	converted, err := transactionToRPC(SimulationRequest{Sender: sender, Transaction: transaction})
	if err != nil {
		t.Fatalf("transactionToRPC() returned an unexpected error: %v", err)
	}
	commands := converted.GetKind().GetProgrammableTransaction().GetCommands()
	if len(commands) != 2 || commands[1].GetMakeMoveVector() == nil || commands[1].GetMakeMoveVector().GetElementType() != "0x2::coin::Coin<0x2::sui::SUI>" {
		t.Fatalf("transactionToRPC() commands = %+v", commands)
	}
}
