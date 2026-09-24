//go:build integration

package sui_test

import (
	"context"
	"testing"
	"time"

	"github.com/k4k3ru-hub/onchain/go/core"
	"github.com/k4k3ru-hub/onchain/go/sui"
)

// TestTestnetConnection verifies Testnet identification and matching GraphQL/gRPC checkpoints.
//
// Version:
//   - 2026-09-24: Added.
func TestTestnetConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	rpc, err := sui.NewRPCClient(ctx, sui.RPCConfig{URL: "https://graphql.testnet.sui.io/graphql"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("network", func(t *testing.T) {
		identifier, err := rpc.ChainIdentifier(ctx)
		if err != nil {
			t.Fatal(err)
		}
		network, err := sui.ResolveNetwork(identifier)
		if err != nil || network != core.NetworkTestnet {
			t.Fatalf("ResolveNetwork() = %q, %v, want testnet", network, err)
		}
		t.Logf("network=%s chain_identifier=%s", network, identifier)
	})
	t.Run("checkpoints", func(t *testing.T) {
		sequence, err := rpc.LatestCheckpointSequenceNumber(ctx)
		if err != nil {
			t.Fatal(err)
		}
		checkpoint, err := rpc.CheckpointBySequenceNumber(ctx, sequence)
		if err != nil {
			t.Fatal(err)
		}
		if checkpoint.SequenceNumber != sequence || checkpoint.Digest.IsZero() || checkpoint.Timestamp.IsZero() {
			t.Fatalf("invalid GraphQL checkpoint: %+v", checkpoint)
		}
		byDigest, err := rpc.CheckpointByDigest(ctx, checkpoint.Digest)
		if err != nil {
			t.Fatal(err)
		}
		latest, err := rpc.LatestCheckpoint(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if byDigest.SequenceNumber != sequence || latest.SequenceNumber < sequence {
			t.Fatalf("inconsistent GraphQL checkpoint sequences: by_digest=%d latest=%d requested=%d", byDigest.SequenceNumber, latest.SequenceNumber, sequence)
		}
		grpc, err := sui.NewGRPCClient(ctx, sui.GRPCConfig{URL: "https://fullnode.testnet.sui.io:443"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := grpc.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
		viaGRPC, err := grpc.CheckpointBySequenceNumber(ctx, sequence)
		if err != nil {
			t.Fatal(err)
		}
		if viaGRPC.Digest != checkpoint.Digest || !viaGRPC.Timestamp.Equal(checkpoint.Timestamp) || viaGRPC.Epoch != checkpoint.Epoch || viaGRPC.NetworkTotalTransactions != checkpoint.NetworkTotalTransactions {
			t.Fatalf("checkpoint differs between GraphQL and gRPC: graphql=%+v grpc=%+v", checkpoint, viaGRPC)
		}
		t.Logf("checkpoint=%d digest=%s graphql_grpc_match=true", sequence, checkpoint.Digest)
	})
}
