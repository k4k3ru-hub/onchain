package sui

import (
	"context"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
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

type testTransactionExecutionClient struct {
	request *rpcv2.SimulateTransactionRequest
	result  *rpcv2.SimulateTransactionResponse
}

func (c *testTransactionExecutionClient) ExecuteTransaction(context.Context, *rpcv2.ExecuteTransactionRequest, ...grpc.CallOption) (*rpcv2.ExecuteTransactionResponse, error) {
	return nil, nil
}

func (c *testTransactionExecutionClient) SimulateTransaction(_ context.Context, request *rpcv2.SimulateTransactionRequest, _ ...grpc.CallOption) (*rpcv2.SimulateTransactionResponse, error) {
	c.request = request
	return c.result, nil
}

func TestGRPCAdapterSimulationRequestsCompleteEvents(t *testing.T) {
	sender, _ := ParseAddress("0x1")
	packageAddress, _ := ParseAddress("0x2")
	builder := NewProgrammableTransactionBuilder()
	_, _ = builder.MoveCall(MoveCall{Package: packageAddress, Module: "fetcher", Function: "quote"})
	transaction, _ := builder.Build()
	success := true
	packageID, module, eventType := packageAddress.String(), "fetcher", packageAddress.String()+"::fetcher::QuoteEvent"
	client := &testTransactionExecutionClient{result: &rpcv2.SimulateTransactionResponse{
		Transaction: &rpcv2.ExecutedTransaction{
			Effects: &rpcv2.TransactionEffects{Status: &rpcv2.ExecutionStatus{Success: &success}},
			Events: &rpcv2.TransactionEvents{Events: []*rpcv2.Event{{
				PackageId: &packageID,
				Module:    &module,
				EventType: &eventType,
				Contents:  &rpcv2.Bcs{Value: []byte{1, 2, 3}},
			}}},
		},
		CommandOutputs: []*rpcv2.CommandResult{{}},
	}}

	result, err := (&grpcAdapter{executionClient: client}).simulateTransaction(context.Background(), SimulationRequest{Sender: sender, Transaction: transaction})
	if err != nil {
		t.Fatalf("simulateTransaction() error = %v", err)
	}
	if len(result.Events) != 1 || len(result.Events[0].BCS) != 3 {
		t.Fatalf("simulateTransaction() events = %+v", result.Events)
	}
	if client.request == nil || client.request.ReadMask == nil || !containsString(client.request.ReadMask.Paths, "transaction.events") || containsString(client.request.ReadMask.Paths, "transaction.events.events") {
		t.Fatalf("simulateTransaction() read mask = %+v", client.request.GetReadMask())
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
