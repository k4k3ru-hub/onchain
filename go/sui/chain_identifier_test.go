package sui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/core"
	"github.com/mr-tron/base58"
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

// TestResolveChainIdentifierRejectsCustomNetwork preserves explicit Sui deployment support.
//
// Version:
//   - 2026-09-25: Added.
func TestResolveChainIdentifierRejectsCustomNetwork(t *testing.T) {
	network := core.Network("custom-testnet")
	if err := network.Validate(); err != nil {
		t.Fatalf("custom network name rejected: %v", err)
	}
	if identifier, err := ResolveChainIdentifier(network); err == nil || identifier != "" {
		t.Fatalf("unsupported network resolved: identifier=%q error=%v", identifier, err)
	}
}

// TestChainIdentifierResponseFormats verifies network resolution for both response formats.
//
// Version:
//   - 2026-09-24: Added.
func TestChainIdentifierResponseFormats(t *testing.T) {
	for _, test := range []struct {
		name       string
		response   string
		identifier ChainIdentifier
		network    core.Network
	}{
		{"legacy mainnet", "35834a8a", ChainIdentifierMainnet, core.NetworkMainnet},
		{"legacy testnet", "4c78adac", ChainIdentifierTestnet, core.NetworkTestnet},
		{"genesis mainnet", "4btiuiMPvEENsttpZC7CZ53DruC3MAgfznDbASZ7DR6S", ChainIdentifierMainnet, core.NetworkMainnet},
		{"genesis testnet", "69WiPg3DAQiwdxfncX6wYQ2siKwAe6L9BZthQea3JNMD", ChainIdentifierTestnet, core.NetworkTestnet},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &fakeCaller{response: map[string]any{"chainIdentifier": test.response}}
			client := composeRPCClient(RPCConfig{}, caller)
			identifier, err := client.ChainIdentifier(nil)
			if err != nil {
				t.Fatal(err)
			}
			if identifier != test.identifier {
				t.Fatalf("ChainIdentifier() = %q, want %q", identifier, test.identifier)
			}
			network, err := ResolveNetwork(identifier)
			if err != nil || network != test.network {
				t.Fatalf("ResolveNetwork() = %q, %v, want %q", network, err, test.network)
			}
		})
	}
}

// TestChainIdentifierRejectsMalformedResponses rejects malformed identifiers and digests.
//
// Version:
//   - 2026-09-24: Added.
func TestChainIdentifierRejectsMalformedResponses(t *testing.T) {
	for name, value := range map[string]any{
		"empty": "", "null": nil, "uppercase short ID": "4C78ADAC", "invalid alphabet": strings.Repeat("0", 44),
		"short digest": base58.Encode(make([]byte, 31)), "long digest": base58.Encode(make([]byte, 33)),
		"truncated short ID": "4c78ada", "wrong JSON type": 123,
	} {
		t.Run(name, func(t *testing.T) {
			client := composeRPCClient(RPCConfig{}, &fakeCaller{response: map[string]any{"chainIdentifier": value}})
			identifier, err := client.ChainIdentifier(nil)
			if err == nil || identifier != "" {
				t.Fatalf("ChainIdentifier() = %q, %v, want an error and no identifier", identifier, err)
			}
		})
	}
}

// TestChainIdentifierPreservesUnknownNetwork keeps unrecognized networks unresolved.
//
// Version:
//   - 2026-09-24: Added.
func TestChainIdentifierPreservesUnknownNetwork(t *testing.T) {
	var digest CheckpointDigest
	copy(digest[:], []byte{0x12, 0x34, 0x56, 0x78})
	client := composeRPCClient(RPCConfig{}, &fakeCaller{response: map[string]any{"chainIdentifier": digest.String()}})
	identifier, err := client.ChainIdentifier(nil)
	if err != nil || identifier != "12345678" {
		t.Fatalf("ChainIdentifier() = %q, %v, want unknown short identifier", identifier, err)
	}
	if network, err := ResolveNetwork(identifier); err == nil || network != "" {
		t.Fatalf("ResolveNetwork() = %q, %v, want unknown network error", network, err)
	}
}

type chainIdentifierErrorCaller struct{ err error }

func (c chainIdentifierErrorCaller) query(context.Context, string, any) error { return c.err }

// TestChainIdentifierPreservesQueryError verifies that caller errors remain inspectable.
//
// Version:
//   - 2026-09-24: Added.
func TestChainIdentifierPreservesQueryError(t *testing.T) {
	client := composeRPCClient(RPCConfig{}, chainIdentifierErrorCaller{err: context.DeadlineExceeded})
	if _, err := client.ChainIdentifier(nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ChainIdentifier() error = %v, want deadline exceeded", err)
	}
}
