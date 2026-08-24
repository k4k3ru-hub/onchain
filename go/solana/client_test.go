package solana

import (
	"context"
	"testing"
)

type fakeRPCDependency struct {
	blockHeight           BlockHeight
	blockHeightErr        error
	blockHeightContext    context.Context
	blockHeightCommitment Commitment
	genesisHash           Hash
	genesisHashErr        error
	slot                  Slot
	slotErr               error
	statuses              []*rpcSignatureStatus
	statusErr             error
	slotContext           context.Context
	statusContext         context.Context
	commitment            Commitment
	statusSignatures      []Signature
}

func (f *fakeRPCDependency) getBlock(context.Context, Slot, Commitment) (*Block, error) {
	return &Block{}, nil
}
func (f *fakeRPCDependency) getAccount(context.Context, Address, Commitment) (*Account, error) {
	return &Account{}, nil
}
func (f *fakeRPCDependency) getTransaction(context.Context, Signature, Commitment) (*Transaction, error) {
	return &Transaction{}, nil
}
func (f *fakeRPCDependency) getAddressSignatures(context.Context, Address, Commitment, int) ([]addressSignature, error) {
	return nil, nil
}

func (f *fakeRPCDependency) getGenesisHash(context.Context) (Hash, error) {
	return f.genesisHash, f.genesisHashErr
}

func (f *fakeRPCDependency) getBlockHeight(ctx context.Context, commitment Commitment) (BlockHeight, error) {
	f.blockHeightContext = ctx
	f.blockHeightCommitment = commitment
	return f.blockHeight, f.blockHeightErr
}

func (f *fakeRPCDependency) getSlot(ctx context.Context, commitment Commitment) (Slot, error) {
	f.slotContext = ctx
	f.commitment = commitment
	return f.slot, f.slotErr
}

func (f *fakeRPCDependency) getSignatureStatuses(ctx context.Context, signatures []Signature) ([]*rpcSignatureStatus, error) {
	f.statusContext = ctx
	f.statusSignatures = append([]Signature(nil), signatures...)
	return f.statuses, f.statusErr
}

func TestComposeRPCClient(t *testing.T) {
	dependency := &fakeRPCDependency{}
	config := RPCConfig{
		URL:        "https://example.com",
		Commitment: CommitmentFinalized,
	}

	client := composeRPCClient(config, dependency)
	if client == nil {
		t.Fatal("composeRPCClient() = nil")
	}
	if client.slotProvider != dependency {
		t.Error("composeRPCClient() did not compose slot provider")
	}
	if client.blockHeightProvider != dependency {
		t.Error("composeRPCClient() did not compose block height provider")
	}
	if client.genesisHashProvider != dependency {
		t.Error("composeRPCClient() did not compose genesis hash provider")
	}
	if client.blockProvider != dependency || client.accountProvider != dependency || client.transactionProvider != dependency {
		t.Error("composeRPCClient() did not compose block, account, and transaction providers")
	}
	if client.signatureStatusProvider != dependency {
		t.Error("composeRPCClient() did not compose signature status provider")
	}
	if client.config != config {
		t.Errorf("composeRPCClient() config = %+v, want %+v", client.config, config)
	}
}

func TestRPCConfigValidate(t *testing.T) {
	config := RPCConfig{
		URL:        " https://example.com ",
		Commitment: CommitmentFinalized,
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("RPCConfig.Validate() error = %v", err)
	}
}

func TestRPCConfigRejectsInvalidInput(t *testing.T) {
	tests := []RPCConfig{
		{Commitment: CommitmentFinalized},
		{URL: "https://example.com"},
		{URL: "https://example.com", Commitment: Commitment("invalid")},
	}
	for _, config := range tests {
		if err := config.Validate(); err == nil {
			t.Errorf("RPCConfig(%+v).Validate() error = nil, want error", config)
		}
	}
}

func TestNewRPCClient(t *testing.T) {
	client, err := NewRPCClient(nil, RPCConfig{
		URL:        "https://example.com",
		Commitment: CommitmentFinalized,
	})
	if err != nil {
		t.Fatalf("NewRPCClient() error = %v", err)
	}
	if client == nil || client.blockProvider == nil || client.accountProvider == nil || client.transactionProvider == nil || client.blockHeightProvider == nil || client.genesisHashProvider == nil || client.slotProvider == nil || client.signatureStatusProvider == nil {
		t.Fatalf("NewRPCClient() = %+v, want composed providers", client)
	}
}
