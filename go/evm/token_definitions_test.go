package evm

import (
	"github.com/ethereum/go-ethereum/common"
	"testing"
)

// TestTokenDefinitionsAreScopedAndDetached verifies network identity and value ownership.
//
// Version:
//   - 2026-09-17: Added.
func TestTokenDefinitionsAreScopedAndDetached(t *testing.T) {
	address := common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")
	value, ok := LookupTokenMetadata(ChainIDBaseMainnet, address)
	if !ok || value.Symbol != "USDC" || value.Decimals != 6 {
		t.Fatalf("metadata=%+v found=%t", value, ok)
	}
	value.Symbol = "changed"
	again, _ := LookupTokenMetadata(ChainIDBaseMainnet, address)
	if again.Symbol != "USDC" {
		t.Fatal("caller mutated definition")
	}
	for _, chain := range []ChainID{ChainIDBaseSepolia, ChainIDEthereumMainnet, 0} {
		if _, ok := LookupTokenMetadata(chain, address); ok {
			t.Fatalf("definition leaked to chain=%d", chain)
		}
	}
	if _, ok := LookupTokenMetadata(ChainIDBaseMainnet, common.Address{}); ok {
		t.Fatal("native currency treated as erc20")
	}
	for key, token := range tokenDefinitions {
		if !common.IsHexAddress(token.Address) || common.HexToAddress(token.Address) != key.address || token.Symbol == "" {
			t.Fatalf("invalid definition=%+v", token)
		}
	}
}
