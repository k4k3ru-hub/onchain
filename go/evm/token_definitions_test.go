package evm

import (
	"github.com/ethereum/go-ethereum/common"
	"testing"
)

// TestTokenDefinitionsAreScopedAndDetached verifies network identity and value ownership.
//
// Version:
//   - 2026-09-17: Added.
//   - 2026-09-20: Reject native metadata on unknown chains.
//   - 2026-09-23: Verify optional reference names and detached name values.
func TestTokenDefinitionsAreScopedAndDetached(t *testing.T) {
	address := common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")
	value, ok := LookupTokenMetadata(ChainIDBaseMainnet, address)
	if !ok || value.Symbol != "USDC" || value.Name != "USD Coin" || value.Decimals != 6 {
		t.Fatalf("metadata=%+v found=%t", value, ok)
	}
	value.Symbol = "changed"
	value.Name = "changed"
	again, _ := LookupTokenMetadata(ChainIDBaseMainnet, address)
	if again.Symbol != "USDC" || again.Name != "USD Coin" {
		t.Fatal("caller mutated definition")
	}
	for _, chain := range []ChainID{ChainIDBaseSepolia, ChainIDEthereumMainnet, 0} {
		if _, ok := LookupTokenMetadata(chain, address); ok {
			t.Fatalf("definition leaked to chain=%d", chain)
		}
	}
	if _, ok := LookupTokenMetadata(ChainID(999999), common.Address{}); ok {
		t.Fatal("unknown native currency resolved")
	}
	for key, token := range tokenDefinitions {
		if !common.IsHexAddress(token.Address) || common.HexToAddress(token.Address) != key.address || token.Symbol == "" {
			t.Fatalf("invalid definition=%+v", token)
		}
	}
}

// TestReferenceTokenNames keeps network-specific display names distinct from symbols.
//
// Version:
//   - 2026-09-23: Added.
func TestReferenceTokenNames(t *testing.T) {
	for _, tc := range []struct {
		chain         ChainID
		address, name string
	}{
		{ChainIDBaseMainnet, "0x4200000000000000000000000000000000000006", "Wrapped Ether"},
		{ChainIDBaseSepolia, "0x4200000000000000000000000000000000000006", "Wrapped Ether"},
		{ChainIDBaseMainnet, "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", "USD Coin"},
		{ChainIDBaseSepolia, "0x036CbD53842c5426634e7929541eC2318f3dCF7e", "USDC"},
		{ChainIDBaseSepolia, "0xcbb7c0006f23900c38eb856149f799620fcb8a4a", "Coinbase Wrapped BTC"},
		{ChainIDEthereumMainnet, "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", "Wrapped BTC"},
		{ChainIDEthereumMainnet, "0xdAC17F958D2ee523a2206206994597C13D831ec7", "Tether USD"},
		{ChainIDRobinhoodMainnet, "0x39dBED3a2bd333467115dE45665cC57F813C4571", "Pons"},
		{ChainIDRobinhoodMainnet, "0x5fc5360D0400a0Fd4f2af552ADD042D716F1d168", "Global Dollar"},
	} {
		value, ok := LookupTokenMetadata(tc.chain, common.HexToAddress(tc.address))
		if !ok || value.Name != tc.name {
			t.Fatalf("incorrect reference name: chain_id=%d address=%s got=%q want=%q", tc.chain, tc.address, value.Name, tc.name)
		}
	}
	// Empty Name remains supported for caller-defined catalogs and native currency.
	if value, _ := LookupTokenMetadata(ChainIDBaseMainnet, common.Address{}); value.Name != "" {
		t.Fatal("invented native contract name")
	}
}

// TestBaseSepoliaCbBTCDefinition verifies the token identity independently of pool data.
//
// Version:
//   - 2026-09-17: Added.
func TestBaseSepoliaCbBTCDefinition(t *testing.T) {
	address := common.HexToAddress("0xcbb7c0006f23900c38eb856149f799620fcb8a4a")
	token, ok := LookupTokenMetadata(ChainIDBaseSepolia, address)
	if !ok || token.Symbol != "cbBTC" || token.Decimals != 8 || common.HexToAddress(token.Address) != address {
		t.Fatalf("unexpected cbBTC definition: %+v", token)
	}
	if _, ok := LookupTokenMetadata(ChainIDBaseMainnet, address); ok {
		t.Fatal("testnet definition leaked into mainnet")
	}
}

// TestNativeTokenDefinitions verifies explicit chain scoping and detached native metadata.
//
// Version:
//   - 2026-09-20: Added.
func TestNativeTokenDefinitions(t *testing.T) {
	expected := map[ChainID]string{
		ChainIDEthereumMainnet: "ETH", ChainIDEthereumSepolia: "ETH",
		ChainIDBaseMainnet: "ETH", ChainIDBaseSepolia: "ETH",
		ChainIDRobinhoodMainnet: "ETH", ChainIDRobinhoodTestnet: "ETH",
		ChainIDBNBMainnet: "BNB", ChainIDPolygonMainnet: "POL", ChainIDPolygonAmoy: "POL",
	}
	for chain, symbol := range expected {
		t.Run(chain.String(), func(t *testing.T) {
			value, ok := LookupTokenMetadata(chain, common.Address{})
			if !ok || value.Symbol != symbol || value.Decimals != 18 || value.Address != "0x0000000000000000000000000000000000000000" {
				t.Fatalf("unexpected native definition: %+v found=%t", value, ok)
			}
			value.Symbol = "changed"
			again, _ := LookupTokenMetadata(chain, common.Address{})
			if again.Symbol != symbol {
				t.Fatal("native definition mutated")
			}
		})
	}
	for _, chain := range []ChainID{0, 999999} {
		if _, ok := LookupTokenMetadata(chain, common.Address{}); ok {
			t.Fatal("unknown chain resolved as native currency")
		}
	}
}
