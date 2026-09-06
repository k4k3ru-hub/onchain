package evm

import (
	"testing"

	"github.com/k4k3ru-hub/onchain/go/core"
)

// TestRobinhoodChainResolution verifies public chain mappings and invalid networks.
//
// Version:
//   - 2026-09-06: Added.
func TestRobinhoodChainResolution(t *testing.T) {
	chain := core.Chain("robinhood")
	if chain != core.ChainRobinhood || !chain.IsValid() {
		t.Fatal("robinhood chain is not registered")
	}
	if err := chain.Validate(); err != nil {
		t.Fatal(err)
	}
	family, err := chain.ResolveChainFamily()
	if err != nil || family != core.ChainFamilyEVM {
		t.Fatalf("family=%q error=%v", family, err)
	}
	for _, tt := range []struct {
		network core.Network
		id      ChainID
		value   uint64
	}{
		{core.NetworkMainnet, ChainIDRobinhoodMainnet, 4663},
		{core.NetworkTestnet, ChainIDRobinhoodTestnet, 46630},
	} {
		t.Run(string(tt.network), func(t *testing.T) {
			if tt.id.Uint64() != tt.value {
				t.Fatalf("chain ID=%d want=%d", tt.id, tt.value)
			}
			if err := tt.id.Validate(); err != nil {
				t.Fatal(err)
			}
			id, err := ResolveChainID(chain, tt.network)
			if err != nil || id != tt.id {
				t.Fatalf("id=%d error=%v", id, err)
			}
			resolved, err := ResolveChainNetwork(ChainID(tt.value))
			if err != nil || resolved.Chain != chain || resolved.Network != tt.network {
				t.Fatalf("resolved=%+v error=%v", resolved, err)
			}
		})
	}
	for _, network := range []core.Network{core.NetworkSepolia, core.NetworkDevnet, core.NetworkHolesky, core.NetworkAmoy} {
		if _, err := ResolveChainID(chain, network); err == nil {
			t.Fatalf("unsupported network accepted: %q", network)
		}
	}
}
