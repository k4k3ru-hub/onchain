package solana

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRPCClientBlockHeight(t *testing.T) {
	requestContext := context.WithValue(context.Background(), struct{}{}, "request")
	dependency := &fakeRPCDependency{blockHeight: 123456789}
	client := composeRPCClient(RPCConfig{
		URL:        "https://example.com",
		Commitment: CommitmentConfirmed,
	}, dependency)

	blockHeight, err := client.BlockHeight(requestContext)
	if err != nil {
		t.Fatalf("RPCClient.BlockHeight() returned an unexpected error: %v", err)
	}
	if blockHeight != 123456789 {
		t.Fatalf("RPCClient.BlockHeight() = %d, want 123456789", blockHeight)
	}
	if dependency.blockHeightContext != requestContext {
		t.Fatal("RPCClient.BlockHeight() did not forward the request context")
	}
	if dependency.blockHeightCommitment != CommitmentConfirmed {
		t.Fatalf("RPCClient.BlockHeight() commitment = %q, want %q", dependency.blockHeightCommitment, CommitmentConfirmed)
	}
}

func TestRPCClientBlockHeightWrapsProviderError(t *testing.T) {
	wantErr := errors.New("rpc unavailable")
	client := composeRPCClient(RPCConfig{
		URL:        "https://example.com",
		Commitment: CommitmentFinalized,
	}, &fakeRPCDependency{blockHeightErr: wantErr})

	_, err := client.BlockHeight(nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("RPCClient.BlockHeight() error = %v, want wrapped error %v", err, wantErr)
	}
}

func TestRPCClientBlockHeightRejectsZero(t *testing.T) {
	client := composeRPCClient(RPCConfig{
		URL:        "https://example.com",
		Commitment: CommitmentFinalized,
	}, &fakeRPCDependency{})

	_, err := client.BlockHeight(nil)
	if err == nil || !strings.Contains(err.Error(), "block_height=empty") {
		t.Fatalf("RPCClient.BlockHeight() error = %v, want block_height=empty", err)
	}
}
