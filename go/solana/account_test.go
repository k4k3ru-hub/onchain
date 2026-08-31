package solana

import (
	"context"
	"testing"
)

type fakeAccountSnapshotProvider struct {
	snapshot   *AccountSnapshot
	err        error
	ctx        context.Context
	commitment Commitment
}

func (f *fakeAccountSnapshotProvider) getAccountSnapshot(ctx context.Context, _ []Address, commitment Commitment) (*AccountSnapshot, error) {
	f.ctx = ctx
	f.commitment = commitment
	return f.snapshot, f.err
}

func TestRPCClientAccountSnapshot(t *testing.T) {
	address := Address{1}
	data := []byte{2}
	provider := &fakeAccountSnapshotProvider{snapshot: &AccountSnapshot{Slot: 10, Accounts: []*Account{{Address: address, Data: data}}}}
	client := &RPCClient{config: RPCConfig{Commitment: CommitmentFinalized}, accountSnapshotProvider: provider}

	snapshot, err := client.AccountSnapshot(nil, []Address{address})
	if err != nil {
		t.Fatalf("AccountSnapshot() error = %v", err)
	}
	if snapshot.Slot != 10 || len(snapshot.Accounts) != 1 || snapshot.Accounts[0].Address != address {
		t.Fatalf("AccountSnapshot() = %+v", snapshot)
	}
	if provider.ctx == nil || provider.commitment != CommitmentFinalized {
		t.Fatalf("provider request = context:%v commitment:%q", provider.ctx, provider.commitment)
	}
	data[0] = 9
	if snapshot.Accounts[0].Data[0] != 2 {
		t.Fatal("AccountSnapshot() did not clone account data")
	}
}

func TestRPCClientAccountSnapshotRejectsInvalidState(t *testing.T) {
	address := Address{1}
	tests := []*RPCClient{
		nil,
		{},
		{accountSnapshotProvider: &fakeAccountSnapshotProvider{err: context.Canceled}},
		{accountSnapshotProvider: &fakeAccountSnapshotProvider{snapshot: nil}},
		{accountSnapshotProvider: &fakeAccountSnapshotProvider{snapshot: &AccountSnapshot{Accounts: []*Account{{Address: address}}}}},
		{accountSnapshotProvider: &fakeAccountSnapshotProvider{snapshot: &AccountSnapshot{Slot: 1}}},
	}
	for _, client := range tests {
		if _, err := client.AccountSnapshot(context.Background(), []Address{address}); err == nil {
			t.Fatal("AccountSnapshot() error = nil, want error")
		}
	}
	client := &RPCClient{accountSnapshotProvider: &fakeAccountSnapshotProvider{snapshot: &AccountSnapshot{Slot: 1, Accounts: []*Account{}}}}
	if _, err := client.AccountSnapshot(context.Background(), nil); err == nil {
		t.Fatal("AccountSnapshot(nil) error = nil, want error")
	}
	if _, err := client.AccountSnapshot(context.Background(), []Address{{}}); err == nil {
		t.Fatal("AccountSnapshot(zero) error = nil, want error")
	}
}
