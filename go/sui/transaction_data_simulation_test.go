package sui

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
)

type transactionDataSimulationStub struct {
	rpcv2.TransactionExecutionServiceClient
	request  *rpcv2.SimulateTransactionRequest
	response *rpcv2.SimulateTransactionResponse
	err      error
}

func (s *transactionDataSimulationStub) SimulateTransaction(_ context.Context, request *rpcv2.SimulateTransactionRequest, _ ...grpc.CallOption) (*rpcv2.SimulateTransactionResponse, error) {
	s.request = request
	return s.response, s.err
}
func transactionDataTestPointer[T any](value T) *T { return &value }
func transactionDataSimulationResponse(data []byte) *rpcv2.SimulateTransactionResponse {
	return &rpcv2.SimulateTransactionResponse{Transaction: &rpcv2.ExecutedTransaction{
		Transaction: &rpcv2.Transaction{Bcs: &rpcv2.Bcs{Value: append([]byte{}, data...)}},
		Effects: &rpcv2.TransactionEffects{Status: &rpcv2.ExecutionStatus{Success: transactionDataTestPointer(true)}, GasUsed: &rpcv2.GasCostSummary{
			ComputationCost: transactionDataTestPointer(uint64(100)), StorageCost: transactionDataTestPointer(uint64(20)), StorageRebate: transactionDataTestPointer(uint64(5)), NonRefundableStorageFee: transactionDataTestPointer(uint64(1)),
		}},
		BalanceChanges: []*rpcv2.BalanceChange{{Address: transactionDataTestPointer("0x1"), CoinType: transactionDataTestPointer("0x2::sui::SUI"), Amount: transactionDataTestPointer("-115")}},
	}}
}

// TestTransactionDataSimulationComposition verifies public and injected composition roots.
//
// Version:
//   - 2026-09-24: Added.
func TestTransactionDataSimulationComposition(t *testing.T) {
	client, err := NewGRPCClient(nil, GRPCConfig{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if client.transactionDataSimulationProvider == nil || client.simulationProvider == nil || client.checkpointProvider == nil {
		t.Fatal("missing composed provider")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	transaction := transactionTestData(t)
	raw, err := transaction.MarshalBCS()
	if err != nil {
		t.Fatal(err)
	}
	stub := &transactionDataSimulationStub{response: transactionDataSimulationResponse(raw)}
	client = composeGRPCClient(GRPCConfig{}, &grpcAdapter{executionClient: stub}, nil)
	result, err := client.SimulateTransactionData(nil, transaction)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.TransactionBytes, raw) || result.GasCost.ComputationCost.Uint64() != 100 || len(result.BalanceChanges) != 1 || result.BalanceChanges[0].Amount.Int64() != -115 {
		t.Fatal("lost simulation effects")
	}
	if stub.request.Checks == nil || stub.request.GetChecks() != rpcv2.SimulateTransactionRequest_ENABLED || stub.request.DoGasSelection == nil || stub.request.GetDoGasSelection() {
		t.Fatal("checks or gas selection changed")
	}
	if !bytes.Equal(stub.request.Transaction.Bcs.Value, raw) {
		t.Fatal("simulated different transaction")
	}
	wantMask := []string{"transaction.transaction.bcs", "transaction.effects", "transaction.events", "transaction.balance_changes"}
	if !reflect.DeepEqual(stub.request.ReadMask.Paths, wantMask) {
		t.Fatal("missing requested fields")
	}
	stub.response.Transaction.Transaction.Bcs.Value[0] = 99
	if result.TransactionBytes[0] != 0 {
		t.Fatal("result shares response memory")
	}
}

// TestTransactionDataSimulationRejectsInvalidResponses verifies failure and identity handling.
//
// Version:
//   - 2026-09-24: Added.
func TestTransactionDataSimulationRejectsInvalidResponses(t *testing.T) {
	transaction := transactionTestData(t)
	raw, err := transaction.MarshalBCS()
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*rpcv2.SimulateTransactionResponse){
		"missing success":      func(r *rpcv2.SimulateTransactionResponse) { r.Transaction.Effects.Status.Success = nil },
		"missing gas":          func(r *rpcv2.SimulateTransactionResponse) { r.Transaction.Effects.GasUsed = nil },
		"missing gas field":    func(r *rpcv2.SimulateTransactionResponse) { r.Transaction.Effects.GasUsed.StorageRebate = nil },
		"modified transaction": func(r *rpcv2.SimulateTransactionResponse) { r.Transaction.Transaction.Bcs.Value[0] = 1 },
		"missing transaction":  func(r *rpcv2.SimulateTransactionResponse) { r.Transaction.Transaction = nil },
	} {
		t.Run(name, func(t *testing.T) {
			response := transactionDataSimulationResponse(raw)
			mutate(response)
			stub := &transactionDataSimulationStub{response: response}
			client := composeGRPCClient(GRPCConfig{}, &grpcAdapter{executionClient: stub}, nil)
			if _, err := client.SimulateTransactionData(nil, transaction); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
	response := transactionDataSimulationResponse(raw)
	response.Transaction.Effects.Status.Success = transactionDataTestPointer(false)
	response.Transaction.Effects.Status.Error = &rpcv2.ExecutionError{Kind: rpcv2.ExecutionError_MOVE_ABORT.Enum()}
	stub := &transactionDataSimulationStub{response: response}
	client := composeGRPCClient(GRPCConfig{}, &grpcAdapter{executionClient: stub}, nil)
	_, err = client.SimulateTransactionData(nil, transaction)
	var executionError *SimulationExecutionError
	if !errors.As(err, &executionError) {
		t.Fatal("lost typed execution failure")
	}
	stub.err = context.DeadlineExceeded
	if _, err := client.SimulateTransactionData(nil, transaction); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lost RPC error chain")
	}
	if _, err := (*GRPCClient)(nil).SimulateTransactionData(nil, transaction); err == nil {
		t.Fatal("accepted nil client")
	}
}
