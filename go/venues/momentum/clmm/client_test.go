package clmm

import (
	"context"
	sui "github.com/k4k3ru-hub/onchain/go/sui"
	"testing"
)

// TestClientComposition verifies the owning constructor wires transports and simulation together.
// No platform request is sent by construction.
//
// Version:
//   - 2026-09-08: Added.
func TestClientComposition(t *testing.T) {
	rpc, err := sui.NewRPCClient(context.Background(), sui.RPCConfig{URL: "https://example.invalid/graphql"})
	if err != nil {
		t.Fatal(err)
	}
	grpc, err := sui.NewGRPCClient(context.Background(), sui.GRPCConfig{URL: "https://example.invalid:443"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := grpc.Close(); err != nil {
			t.Error(err)
		}
	})
	c, err := NewClient(testDeployment(), rpc, grpc)
	if err != nil {
		t.Fatal(err)
	}
	if c.rpc != rpc || c.grpc != grpc || c.quoter == nil || c.quoter.simulator != grpc {
		t.Fatal("incomplete composition")
	}
	if _, err := NewClient(testDeployment(), nil, grpc); err == nil {
		t.Fatal("accepted missing rpc")
	}
}
