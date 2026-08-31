package evm

import (
	"strings"
	"testing"

	myCore "github.com/k4k3ru-hub/onchain/go/core"
)

func TestCatalogNormalizesResolvesAndRejectsDuplicates(t *testing.T) {
	entry := validPoolMetadata()
	catalog, err := NewCatalog([]PoolMetadata{entry})
	if err != nil {
		t.Fatal(err)
	}
	reference := entry.Reference.Normalize()
	got, ok := catalog.Resolve(reference)
	if !ok || !strings.EqualFold(got.Address, entry.Address) || got.Reference.PoolID != got.Address || got.Token0.Symbol != "WETH" {
		t.Fatalf("Resolve() metadata=%+v found=%t", got, ok)
	}
	if _, err := NewCatalog([]PoolMetadata{entry, entry}); err == nil {
		t.Fatal("NewCatalog() duplicate error=nil")
	}
}

func TestCatalogRejectsNonEVMAndInvalidMetadata(t *testing.T) {
	entry := validPoolMetadata()
	entry.Reference.Chain = myCore.ChainSolana
	if _, err := NewCatalog([]PoolMetadata{entry}); err == nil {
		t.Fatal("NewCatalog() non-EVM error=nil")
	}
	entry = validPoolMetadata()
	entry.Token1.Address = entry.Token0.Address
	if _, err := NewCatalog([]PoolMetadata{entry}); err == nil {
		t.Fatal("NewCatalog() duplicate token error=nil")
	}
}

func TestCatalogAcceptsAddresslessPoolID(t *testing.T) {
	entry := validPoolMetadata()
	entry.Reference.Protocol = "uniswap-v4"
	entry.Reference.PoolID = "0xabcdef0000000000000000000000000000000000000000000000000000000001"
	entry.Address = ""
	entry.Deployment.PoolManager = "0x0000000000000000000000000000000000000004"

	catalog, err := NewCatalog([]PoolMetadata{entry})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := catalog.Resolve(entry.Reference)
	if !ok || got.Address != "" || got.Reference.PoolID != entry.Reference.PoolID {
		t.Fatalf("Resolve() metadata=%+v found=%t", got, ok)
	}
}

func validPoolMetadata() PoolMetadata {
	return PoolMetadata{
		Reference: myCore.PoolReference{Chain: myCore.ChainBase, Network: myCore.NetworkMainnet, Protocol: " Uniswap-V3 ", PoolID: "0xabcdef0000000000000000000000000000000001"},
		Address:   "0xabcdef0000000000000000000000000000000001",
		Token0:    TokenMetadata{Symbol: " weth ", Address: "0x0000000000000000000000000000000000000002", Decimals: 18},
		Token1:    TokenMetadata{Symbol: "USDC", Address: "0x0000000000000000000000000000000000000003", Decimals: 6},
		Fee:       500,
	}
}
