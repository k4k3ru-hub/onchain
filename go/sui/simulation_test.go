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
