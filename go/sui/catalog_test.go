package sui

import (
	"strings"
	"testing"

	myCore "github.com/k4k3ru-hub/onchain/go/core"
)

func TestCatalogNormalizesAndResolvesSuiPool(t *testing.T) {
	address, _ := ParseAddress("0x9")
	entry := PoolMetadata{
		Reference:      myCore.PoolReference{Chain: myCore.ChainSui, Network: myCore.NetworkMainnet, Protocol: " Cetus ", PoolID: "0x9"},
		Address:        address,
		InitialVersion: 3,
		TokenA:         TokenMetadata{Symbol: " sui ", CoinType: " 0x2::sui::SUI ", Decimals: 9},
		TokenB:         TokenMetadata{Symbol: " usdc ", CoinType: " 0x3::usdc::USDC ", Decimals: 6},
	}
	catalog, err := NewCatalog([]PoolMetadata{entry})
	if err != nil {
		t.Fatalf("NewCatalog() returned an unexpected error: %v", err)
	}
	resolved, ok := catalog.Resolve(entry.Reference)
	if !ok || resolved.Reference.Protocol != "cetus" || resolved.Reference.PoolID != address.String() || resolved.TokenA.Symbol != "SUI" || resolved.TokenA.CoinType != "0x2::sui::SUI" {
		t.Fatalf("Resolve() = %+v, %t", resolved, ok)
	}
	entries := catalog.Entries()
	entries[0].TokenA.Symbol = "CHANGED"
	resolved, _ = catalog.Resolve(entry.Reference)
	if resolved.TokenA.Symbol != "SUI" {
		t.Fatal("Entries() exposed catalog-owned metadata")
	}
}

func TestCatalogRejectsInvalidSuiPools(t *testing.T) {
	address, _ := ParseAddress("0x9")
	valid := PoolMetadata{
		Reference:      myCore.PoolReference{Chain: myCore.ChainSui, Network: myCore.NetworkMainnet, Protocol: "cetus", PoolID: address.String()},
		Address:        address,
		InitialVersion: 3,
		TokenA:         TokenMetadata{Symbol: "SUI", CoinType: "0x2::sui::SUI", Decimals: 9},
		TokenB:         TokenMetadata{Symbol: "USDC", CoinType: "0x3::usdc::USDC", Decimals: 6},
	}
	if _, err := NewCatalog([]PoolMetadata{valid, valid}); err == nil || !strings.Contains(err.Error(), "duplicate=true") {
		t.Fatalf("NewCatalog() duplicate error = %v", err)
	}
	invalidChain := valid
	invalidChain.Reference.Chain = myCore.ChainEthereum
	if _, err := NewCatalog([]PoolMetadata{invalidChain}); err == nil || !strings.Contains(err.Error(), "chain_family=invalid") {
		t.Fatalf("NewCatalog() chain error = %v", err)
	}
	invalidTokens := valid
	invalidTokens.TokenB.CoinType = invalidTokens.TokenA.CoinType
	if _, err := NewCatalog([]PoolMetadata{invalidTokens}); err == nil || !strings.Contains(err.Error(), "duplicate=true") {
		t.Fatalf("NewCatalog() token error = %v", err)
	}
}
