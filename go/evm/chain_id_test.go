package evm

import (
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/core"
)

func TestResolveChainID(t *testing.T) {
	tests := []struct {
		name    string
		chain   core.Chain
		network core.Network
		want    ChainID
	}{
		{
			name:    "ethereum mainnet",
			chain:   core.ChainEthereum,
			network: core.NetworkMainnet,
			want:    ChainIDEthereumMainnet,
		},
		{
			name:    "bnb mainnet",
			chain:   core.ChainBNB,
			network: core.NetworkMainnet,
			want:    ChainIDBNBMainnet,
		},
		{
			name:    "base mainnet",
			chain:   core.ChainBase,
			network: core.NetworkMainnet,
			want:    ChainIDBaseMainnet,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveChainID(test.chain, test.network)
			if err != nil {
				t.Fatalf("ResolveChainID() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("ResolveChainID() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestResolveChainNetwork(t *testing.T) {
	tests := []struct {
		name    string
		chainID ChainID
		chain   core.Chain
		network core.Network
	}{
		{
			name:    "ethereum mainnet",
			chainID: ChainIDEthereumMainnet,
			chain:   core.ChainEthereum,
			network: core.NetworkMainnet,
		},
		{
			name:    "bnb mainnet",
			chainID: ChainIDBNBMainnet,
			chain:   core.ChainBNB,
			network: core.NetworkMainnet,
		},
		{
			name:    "base mainnet",
			chainID: ChainIDBaseMainnet,
			chain:   core.ChainBase,
			network: core.NetworkMainnet,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveChainNetwork(test.chainID)
			if err != nil {
				t.Fatalf("ResolveChainNetwork() error = %v", err)
			}
			if got.Chain != test.chain || got.Network != test.network {
				t.Fatalf("ResolveChainNetwork() = %+v, want chain=%q network=%q", got, test.chain, test.network)
			}
		})
	}
}

func TestChainDefinitionsRoundTrip(t *testing.T) {
	for _, definition := range chainDefinitions {
		chainID, err := ResolveChainID(definition.chain, definition.network)
		if err != nil {
			t.Fatalf("ResolveChainID(%q, %q) error = %v", definition.chain, definition.network, err)
		}
		if chainID != definition.chainID {
			t.Fatalf("ResolveChainID(%q, %q) = %d, want %d", definition.chain, definition.network, chainID, definition.chainID)
		}

		chainNetwork, err := ResolveChainNetwork(chainID)
		if err != nil {
			t.Fatalf("ResolveChainNetwork(%d) error = %v", chainID, err)
		}
		if chainNetwork.Chain != definition.chain || chainNetwork.Network != definition.network {
			t.Fatalf("round trip = %+v, want chain=%q network=%q", chainNetwork, definition.chain, definition.network)
		}
	}
}

func TestResolveChainIDRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		chain     core.Chain
		network   core.Network
		wantError string
	}{
		{name: "empty chain", network: core.NetworkMainnet, wantError: "chain"},
		{name: "invalid chain", chain: core.Chain("unknown"), network: core.NetworkMainnet, wantError: "chain"},
		{name: "empty network", chain: core.ChainEthereum, wantError: "network"},
		{name: "invalid network", chain: core.ChainEthereum, network: core.Network("unknown"), wantError: "network"},
		{name: "solana", chain: core.ChainSolana, network: core.NetworkMainnet, wantError: "chain_family=invalid"},
		{name: "sui", chain: core.ChainSui, network: core.NetworkMainnet, wantError: "chain_family=invalid"},
		{name: "unsupported evm chain", chain: core.ChainPolygon, network: core.NetworkMainnet, wantError: "combination is unsupported"},
		{name: "unsupported network", chain: core.ChainEthereum, network: core.NetworkSepolia, wantError: "combination is unsupported"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ResolveChainID(test.chain, test.network)
			if err == nil {
				t.Fatal("ResolveChainID() error = nil, want error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ResolveChainID() error = %q, want substring %q", err, test.wantError)
			}
		})
	}
}

func TestResolveChainNetworkRejectsInvalidChainID(t *testing.T) {
	tests := []struct {
		name      string
		chainID   ChainID
		wantError string
	}{
		{name: "empty", chainID: 0, wantError: "chain_id=empty"},
		{name: "unsupported", chainID: 999, wantError: "chain_id=invalid"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ResolveChainNetwork(test.chainID)
			if err == nil {
				t.Fatal("ResolveChainNetwork() error = nil, want error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ResolveChainNetwork() error = %q, want substring %q", err, test.wantError)
			}
		})
	}
}

func TestChainIDValues(t *testing.T) {
	tests := []struct {
		chainID ChainID
		value   uint64
		text    string
	}{
		{chainID: ChainIDEthereumMainnet, value: 1, text: "1"},
		{chainID: ChainIDBNBMainnet, value: 56, text: "56"},
		{chainID: ChainIDBaseMainnet, value: 8453, text: "8453"},
	}

	for _, test := range tests {
		if got := test.chainID.Uint64(); got != test.value {
			t.Errorf("ChainID(%d).Uint64() = %d, want %d", test.chainID, got, test.value)
		}
		if got := test.chainID.String(); got != test.text {
			t.Errorf("ChainID(%d).String() = %q, want %q", test.chainID, got, test.text)
		}
		if err := test.chainID.Validate(); err != nil {
			t.Errorf("ChainID(%d).Validate() error = %v", test.chainID, err)
		}
	}
}

func TestChainIDValidateRejectsInvalidChainID(t *testing.T) {
	for _, chainID := range []ChainID{0, 999} {
		if err := chainID.Validate(); err == nil {
			t.Errorf("ChainID(%d).Validate() error = nil, want error", chainID)
		}
	}
}
