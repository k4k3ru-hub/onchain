package solana

import (
	"context"
	"errors"
	"testing"
)

func testSignature(value byte) Signature {
	var signature Signature
	signature[0] = value
	return signature
}

func TestRPCClientTransactionStatuses(t *testing.T) {
	confirmations := uint64(2)
	signatures := []Signature{testSignature(1), testSignature(2), testSignature(3)}
	dependency := &fakeRPCDependency{statuses: []*rpcSignatureStatus{
		{slot: 100, confirmations: &confirmations, confirmationStatus: CommitmentConfirmed},
		nil,
		{slot: 102, confirmationStatus: CommitmentFinalized, failed: true},
	}}
	client := composeRPCClient(RPCConfig{}, dependency)

	statuses, err := client.TransactionStatuses(nil, signatures)
	if err != nil {
		t.Fatalf("TransactionStatuses() error = %v", err)
	}
	if len(statuses) != len(signatures) {
		t.Fatalf("TransactionStatuses() length = %d, want %d", len(statuses), len(signatures))
	}
	if statuses[0] == nil || statuses[0].Signature != signatures[0] || !statuses[0].IsConfirmed() || statuses[0].IsFinalized() || !statuses[0].IsSuccessful() {
		t.Errorf("confirmed status = %+v", statuses[0])
	}
	if statuses[1] != nil {
		t.Errorf("not-found status = %+v, want nil", statuses[1])
	}
	if statuses[2] == nil || statuses[2].IsConfirmed() || statuses[2].IsFinalized() || statuses[2].IsSuccessful() {
		t.Errorf("failed finalized status = %+v", statuses[2])
	}
	if dependency.statusContext == nil {
		t.Error("TransactionStatuses() passed nil context")
	}
	if len(dependency.statusSignatures) != len(signatures) {
		t.Errorf("provider signatures length = %d, want %d", len(dependency.statusSignatures), len(signatures))
	}
}

func TestRPCClientTransactionStatus(t *testing.T) {
	signature := testSignature(1)
	client := &RPCClient{signatureStatusProvider: &fakeRPCDependency{statuses: []*rpcSignatureStatus{
		{slot: 100, confirmationStatus: CommitmentFinalized},
	}}}

	status, err := client.TransactionStatus(context.Background(), signature)
	if err != nil {
		t.Fatalf("TransactionStatus() error = %v", err)
	}
	if status == nil || !status.IsFinalized() {
		t.Errorf("TransactionStatus() = %+v, want finalized status", status)
	}
}

func TestRPCClientTransactionStatusesWrapsError(t *testing.T) {
	statusErr := errors.New("rpc unavailable")
	client := &RPCClient{signatureStatusProvider: &fakeRPCDependency{statusErr: statusErr}}

	_, err := client.TransactionStatuses(context.Background(), []Signature{testSignature(1)})
	if !errors.Is(err, statusErr) {
		t.Fatalf("TransactionStatuses() error = %v, want wrapped RPC error", err)
	}
}

func TestRPCClientTransactionStatusesRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		client     *RPCClient
		signatures []Signature
	}{
		{},
		{client: &RPCClient{}},
		{client: &RPCClient{signatureStatusProvider: &fakeRPCDependency{}}},
		{client: &RPCClient{signatureStatusProvider: &fakeRPCDependency{}}, signatures: []Signature{{}}},
	}
	for _, test := range tests {
		if _, err := test.client.TransactionStatuses(context.Background(), test.signatures); err == nil {
			t.Fatal("TransactionStatuses() error = nil, want error")
		}
	}
}

func TestRPCClientTransactionStatusesRejectsInvalidResponse(t *testing.T) {
	signature := testSignature(1)
	tests := []*fakeRPCDependency{
		{statuses: nil},
		{statuses: []*rpcSignatureStatus{{slot: 1, confirmationStatus: Commitment("invalid")}}},
		{statuses: []*rpcSignatureStatus{{confirmationStatus: CommitmentFinalized}}},
	}
	for _, dependency := range tests {
		client := &RPCClient{signatureStatusProvider: dependency}
		if _, err := client.TransactionStatuses(context.Background(), []Signature{signature}); err == nil {
			t.Fatal("TransactionStatuses() error = nil, want error")
		}
	}
}
