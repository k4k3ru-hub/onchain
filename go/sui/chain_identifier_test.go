package sui

import (
	"context"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/core"
)

func TestChainIdentifier(t *testing.T) {
	caller := &fakeCaller{response: map[string]any{"chainIdentifier": "35834a8a"}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	identifier, err := client.ChainIdentifier(context.Background())
	if err != nil {
		t.Fatalf("ChainIdentifier() returned an unexpected error: %v", err)
	}
	if identifier != ChainIdentifierMainnet {
		t.Fatalf("ChainIdentifier() = %q, want %q", identifier, ChainIdentifierMainnet)
	}
	network, err := ResolveNetwork(identifier)
	if err != nil {
		t.Fatalf("ResolveNetwork() returned an unexpected error: %v", err)
	}
	if network != core.NetworkMainnet {
		t.Fatalf("ResolveNetwork() = %q, want %q", network, core.NetworkMainnet)
	}
}

func TestResolveChainIdentifier(t *testing.T) {
	identifier, err := ResolveChainIdentifier(core.NetworkTestnet)
	if err != nil {
		t.Fatalf("ResolveChainIdentifier() returned an unexpected error: %v", err)
	}
	if identifier != ChainIdentifierTestnet {
		t.Fatalf("ResolveChainIdentifier() = %q, want %q", identifier, ChainIdentifierTestnet)
	}
}
