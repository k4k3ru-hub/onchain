package sui

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
)

type executionRPCStub struct {
	rpcv2.TransactionExecutionServiceClient
	request  *rpcv2.ExecuteTransactionRequest
	response *rpcv2.ExecuteTransactionResponse
	err      error
}

// ExecuteTransaction captures the gRPC request without network access.
//
// Version:
//   - 2026-09-25: Added.
func (s *executionRPCStub) ExecuteTransaction(_ context.Context, request *rpcv2.ExecuteTransactionRequest, _ ...grpc.CallOption) (*rpcv2.ExecuteTransactionResponse, error) {
	s.request = request
	return s.response, s.err
}

// TestExecuteTransaction verifies serialization, signature gating, and response identity.
//
// Version:
//   - 2026-09-25: Added.
func TestExecuteTransaction(t *testing.T) {
	tx := transactionTestData(t)
	raw, err := tx.MarshalBCS()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := transactionTestKey(t).SignTransaction(raw)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := tx.Digest()
	if err != nil {
		t.Fatal(err)
	}
	response := &rpcv2.ExecuteTransactionResponse{Transaction: &rpcv2.ExecutedTransaction{Digest: transactionDataTestPointer(digest.String()), Effects: &rpcv2.TransactionEffects{Status: &rpcv2.ExecutionStatus{Success: transactionDataTestPointer(true)}}}}
	stub := &executionRPCStub{response: response}
	client := composeGRPCClient(GRPCConfig{}, &grpcAdapter{executionClient: stub}, nil)
	result, err := client.ExecuteTransaction(nil, raw, sig)
	if err != nil || result == nil || result.Digest != digest || !result.Success {
		t.Fatal("execution failed", err)
	}
	if !bytes.Equal(stub.request.Transaction.Bcs.Value, raw) || len(stub.request.Signatures) != 1 || !bytes.Equal(stub.request.Signatures[0].Bcs.Value, sig) {
		t.Fatal("changed signed payload")
	}
	if len(stub.request.ReadMask.Paths) != 2 || stub.request.ReadMask.Paths[0] != "digest" || stub.request.ReadMask.Paths[1] != "effects.status" {
		t.Fatal("incorrect read mask")
	}
	// Move failure is a submitted transaction, not an RPC transport failure.
	response.Transaction.Effects.Status.Success = transactionDataTestPointer(false)
	result, err = client.ExecuteTransaction(nil, raw, sig)
	if err != nil || result.Success {
		t.Fatal("lost execution failure", err)
	}
	response.Transaction.Digest = transactionDataTestPointer(TransactionDigest{1}.String())
	if _, err := client.ExecuteTransaction(nil, raw, sig); err == nil {
		t.Fatal("accepted wrong digest")
	}
	stub.response = nil
	if _, err := client.ExecuteTransaction(nil, raw, sig); err == nil {
		t.Fatal("accepted missing effects")
	}
	stub.err = context.DeadlineExceeded
	if _, err := client.ExecuteTransaction(nil, raw, sig); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lost transport error", err)
	}
	stub.request = nil
	sig[1] ^= 1
	if _, err := client.ExecuteTransaction(nil, raw, sig); err == nil || stub.request != nil {
		t.Fatal("sent invalid signature")
	}
	if _, err := (*GRPCClient)(nil).ExecuteTransaction(nil, raw, sig); err == nil {
		t.Fatal("accepted nil client")
	}
	composed, err := NewGRPCClient(nil, GRPCConfig{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if composed.transactionExecutionProvider == nil {
		t.Fatal("missing execution provider")
	}
	if err := composed.Close(); err != nil {
		t.Fatal(err)
	}
}
