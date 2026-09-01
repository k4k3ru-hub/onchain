package solana

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	solanaSDK "github.com/gagliardetto/solana-go"
	solanaRPC "github.com/gagliardetto/solana-go/rpc"
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

func TestConvertAccountKeysIncludesLoadedAddressesInRuntimeOrder(t *testing.T) {
	static := solanaSDK.NewWallet().PublicKey()
	writable := solanaSDK.NewWallet().PublicKey()
	readOnly := solanaSDK.NewWallet().PublicKey()
	transaction := &solanaSDK.Transaction{Message: solanaSDK.Message{AccountKeys: solanaSDK.PublicKeySlice{static}}}
	meta := &solanaRPC.TransactionMeta{LoadedAddresses: solanaRPC.LoadedAddresses{
		Writable: solanaSDK.PublicKeySlice{writable},
		ReadOnly: solanaSDK.PublicKeySlice{readOnly},
	}}

	got := convertAccountKeys(transaction, meta)
	if len(got) != 3 || got[0].String() != static.String() || got[1].String() != writable.String() || got[2].String() != readOnly.String() {
		t.Fatalf("convertAccountKeys() = %v", got)
	}
}

func (f *fakeRPCDependency) getBlock(context.Context, Slot, Commitment) (*Block, error) {
	return &Block{}, nil
}
func (f *fakeRPCDependency) getAccount(context.Context, Address, Commitment) (*Account, error) {
	return &Account{}, nil
}
func (f *fakeRPCDependency) getAccountSnapshot(context.Context, []Address, Commitment) (*AccountSnapshot, error) {
	return &AccountSnapshot{Slot: 1, Accounts: []*Account{}}, nil
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
	if client.accountSnapshotProvider != dependency {
		t.Error("composeRPCClient() did not compose account snapshot provider")
	}
	if client.signatureStatusProvider != dependency {
		t.Error("composeRPCClient() did not compose signature status provider")
	}
	if client.config != config {
		t.Errorf("composeRPCClient() config = %+v, want %+v", client.config, config)
	}
}

func TestSDKRPCAdapterSanitizeErrorRedactsEndpointCredentials(t *testing.T) {
	endpoint := "https://user:password@mainnet.helius-rpc.com/?api-key=secret-value"
	underlying := errors.New("context deadline exceeded")
	adapter := &sdkRPCAdapter{endpoint: endpoint}

	err := adapter.sanitizeError(fmt.Errorf("rpc call on %s: %w", endpoint, underlying))
	if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "password") {
		t.Fatalf("sanitizeError() exposed credentials: %v", err)
	}
	if !strings.Contains(err.Error(), "https://mainnet.helius-rpc.com/") {
		t.Fatalf("sanitizeError() = %v", err)
	}
	if !errors.Is(err, underlying) {
		t.Fatal("sanitizeError() did not preserve wrapped error")
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
	if client == nil || client.blockProvider == nil || client.accountProvider == nil || client.accountSnapshotProvider == nil || client.transactionProvider == nil || client.blockHeightProvider == nil || client.genesisHashProvider == nil || client.slotProvider == nil || client.signatureStatusProvider == nil {
		t.Fatalf("NewRPCClient() = %+v, want composed providers", client)
	}
}
