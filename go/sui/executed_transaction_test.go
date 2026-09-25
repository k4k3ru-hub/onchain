package sui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type executedTransactionRPCStub struct {
	rpcv2.LedgerServiceClient
	request  *rpcv2.GetTransactionRequest
	response *rpcv2.GetTransactionResponse
	err      error
}

// GetTransaction captures a ledger read without accessing a live node.
//
// Version:
//   - 2026-09-25: Added.
func (s *executedTransactionRPCStub) GetTransaction(_ context.Context, request *rpcv2.GetTransactionRequest, _ ...grpc.CallOption) (*rpcv2.GetTransactionResponse, error) {
	s.request = request
	return s.response, s.err
}

func executedTransactionFixture(t *testing.T) (TransactionDigest, *rpcv2.GetTransactionResponse) {
	t.Helper()
	tx := transactionTestData(t)
	raw, err := tx.MarshalBCS()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := tx.Digest()
	if err != nil {
		t.Fatal(err)
	}
	p := transactionDataTestPointer[string]
	u := transactionDataTestPointer[uint64]
	return digest, &rpcv2.GetTransactionResponse{Transaction: &rpcv2.ExecutedTransaction{
		Digest: p(digest.String()), Transaction: &rpcv2.Transaction{Bcs: &rpcv2.Bcs{Value: raw}},
		Effects: &rpcv2.TransactionEffects{
			TransactionDigest: p(digest.String()), Status: &rpcv2.ExecutionStatus{Success: transactionDataTestPointer(true)},
			GasUsed:      &rpcv2.GasCostSummary{ComputationCost: u(1000), StorageCost: u(2000), StorageRebate: u(4000), NonRefundableStorageFee: u(40)},
			EventsDigest: p(TransactionDigest{3}.String()),
		},
		Checkpoint: u(9007199254740993), TransactionIndex: u(0), Timestamp: timestamppb.New(time.Unix(1_790_000_000, 0)),
		BalanceChanges: []*rpcv2.BalanceChange{{Address: p(tx.Sender.String()), CoinType: p("0x2::sui::SUI"), Amount: p("-9007199254740993")}},
		Events:         &rpcv2.TransactionEvents{Events: []*rpcv2.Event{{PackageId: p("0x42"), Module: p("pool"), Sender: p(tx.Sender.String()), EventType: p("0x42::pool::SwapEvent"), Contents: &rpcv2.Bcs{Value: []byte{1, 2, 3}}}}},
	}}
}

// TestExecutedTransaction verifies complete evidence reads, exact amounts, and missing transactions.
//
// Version:
//   - 2026-09-25: Added.
func TestExecutedTransaction(t *testing.T) {
	digest, response := executedTransactionFixture(t)
	stub := &executedTransactionRPCStub{response: response}
	client := composeGRPCClient(GRPCConfig{}, &grpcAdapter{ledgerClient: stub}, nil)
	result, err := client.ExecutedTransaction(nil, digest)
	if err != nil {
		t.Fatal(err)
	}
	if !stub.request.ReadMask.IsValid(&rpcv2.ExecutedTransaction{}) || stub.request.GetDigest() != digest.String() {
		t.Fatal("invalid ledger request")
	}
	paths := strings.Join(stub.request.ReadMask.Paths, ",")
	for _, required := range []string{"transaction.bcs", "effects.gas_used", "balance_changes", "events", "checkpoint", "timestamp"} {
		if !strings.Contains(paths, required) {
			t.Fatalf("missing read field %q", required)
		}
	}
	if result.Effects.Digest != digest || !result.Effects.Successful || uint64(*result.Effects.Checkpoint) != 9007199254740993 || result.Effects.TransactionIndex == nil || *result.Effects.TransactionIndex != 0 {
		t.Fatal("lost transaction identity or checkpoint precision")
	}
	if len(result.Effects.BalanceChanges) != 1 || result.Effects.BalanceChanges[0].Amount.String() != "-9007199254740993" || result.Effects.GasCost.StorageRebate.String() != "4000" {
		t.Fatal("lost signed integer amounts or storage rebate")
	}
	if len(result.Events) != 1 || !bytes.Equal(result.Events[0].BCS, []byte{1, 2, 3}) || !bytes.Equal(result.TransactionBytes, response.Transaction.Transaction.Bcs.Value) {
		t.Fatal("lost transaction or event bytes")
	}
	result.TransactionBytes[0] ^= 1
	result.Events[0].BCS[0] ^= 1
	if response.Transaction.Transaction.Bcs.Value[0] == result.TransactionBytes[0] || response.Transaction.Events.Events[0].Contents.Value[0] == result.Events[0].BCS[0] {
		t.Fatal("aliased transport buffers")
	}
	response.Transaction.Effects.Status.Success = transactionDataTestPointer(false)
	response.Transaction.Events, response.Transaction.Effects.EventsDigest = nil, nil
	result, err = client.ExecutedTransaction(nil, digest)
	if err != nil || result.Effects.Successful || result.Effects.GasCost.ComputationCost.String() != "1000" {
		t.Fatal("lost failed transaction gas costs", err)
	}
	response.Transaction.Checkpoint, response.Transaction.Timestamp, response.Transaction.TransactionIndex = nil, nil, nil
	result, err = client.ExecutedTransaction(nil, digest)
	if err != nil || result.Effects.Checkpoint != nil || result.Effects.Timestamp != nil {
		t.Fatal("invented checkpoint finality", err)
	}
	stub.err = status.Error(codes.NotFound, "not found")
	if result, err := client.ExecutedTransaction(nil, digest); result != nil || err != nil {
		t.Fatal("did not preserve absent ledger result", err)
	}
	stub.err = context.DeadlineExceeded
	if _, err := client.ExecutedTransaction(nil, digest); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lost transport error", err)
	}
	stub.err = status.Error(codes.Unavailable, "unavailable")
	if _, err := client.ExecutedTransaction(nil, digest); status.Code(err) != codes.Unavailable {
		t.Fatal("classified unavailable node as absent transaction", err)
	}
	stub.request = nil
	if _, err := client.ExecutedTransaction(nil, TransactionDigest{}); err == nil || stub.request != nil {
		t.Fatal("queried empty digest")
	}
	if _, err := (*GRPCClient)(nil).ExecutedTransaction(nil, digest); err == nil {
		t.Fatal("accepted nil client")
	}
	composed, err := NewGRPCClient(nil, GRPCConfig{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if composed.executedTransactionProvider == nil {
		t.Fatal("missing ledger read provider")
	}
	if err := composed.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestExecutedTransactionRejectsIncompleteEvidence prevents malformed reads from becoming fills.
//
// Version:
//   - 2026-09-25: Added.
func TestExecutedTransactionRejectsIncompleteEvidence(t *testing.T) {
	digest, fixture := executedTransactionFixture(t)
	for _, test := range []struct {
		name   string
		mutate func(*rpcv2.GetTransactionResponse)
	}{
		{"missing transaction", func(r *rpcv2.GetTransactionResponse) { r.Transaction = nil }},
		{"wrong digest", func(r *rpcv2.GetTransactionResponse) {
			r.Transaction.Digest = transactionDataTestPointer(TransactionDigest{1}.String())
		}},
		{"wrong effects", func(r *rpcv2.GetTransactionResponse) { r.Transaction.Effects.TransactionDigest = nil }},
		{"wrong bytes", func(r *rpcv2.GetTransactionResponse) { r.Transaction.Transaction.Bcs.Value[0] ^= 1 }},
		{"missing bytes", func(r *rpcv2.GetTransactionResponse) { r.Transaction.Transaction = nil }},
		{"missing status", func(r *rpcv2.GetTransactionResponse) { r.Transaction.Effects.Status.Success = nil }},
		{"missing gas", func(r *rpcv2.GetTransactionResponse) { r.Transaction.Effects.GasUsed.StorageRebate = nil }},
		{"missing time", func(r *rpcv2.GetTransactionResponse) { r.Transaction.Timestamp = nil }},
		{"bad time", func(r *rpcv2.GetTransactionResponse) { r.Transaction.Timestamp.Nanos = -1 }},
		{"missing events", func(r *rpcv2.GetTransactionResponse) { r.Transaction.Events = nil }},
		{"missing event bytes", func(r *rpcv2.GetTransactionResponse) { r.Transaction.Events.Events[0].Contents = nil }},
		{"invalid amount", func(r *rpcv2.GetTransactionResponse) {
			r.Transaction.BalanceChanges[0].Amount = transactionDataTestPointer("1.1")
		}},
		{"null balance", func(r *rpcv2.GetTransactionResponse) { r.Transaction.BalanceChanges[0] = nil }},
		{"invalid type", func(r *rpcv2.GetTransactionResponse) {
			r.Transaction.Events.Events[0].EventType = transactionDataTestPointer("bad")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := proto.Clone(fixture).(*rpcv2.GetTransactionResponse)
			test.mutate(response)
			client := composeGRPCClient(GRPCConfig{}, &grpcAdapter{ledgerClient: &executedTransactionRPCStub{response: response}}, nil)
			if _, err := client.ExecutedTransaction(nil, digest); err == nil {
				t.Fatal("accepted incomplete evidence")
			}
		})
	}
}
