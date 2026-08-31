package solana

import (
	"testing"

	myCore "github.com/k4k3ru-hub/onchain/go/core"
)

func TestCatalogNormalizesResolvesAndRejectsDuplicates(t *testing.T) {
	entry := validPoolMetadata(t)
	catalog, err := NewCatalog([]PoolMetadata{entry})
	if err != nil {
		t.Fatal(err)
	}
	reference := entry.Reference.Normalize()
	got, ok := catalog.Resolve(reference)
	if !ok || got.Address != entry.Address || got.TokenA.Symbol != "SOL" {
		t.Fatalf("Resolve() metadata=%+v found=%t", got, ok)
	}
	if _, err := NewCatalog([]PoolMetadata{entry, entry}); err == nil {
		t.Fatal("NewCatalog() duplicate error=nil")
	}
}

func TestCatalogRejectsNonSolanaAndInvalidMetadata(t *testing.T) {
	entry := validPoolMetadata(t)
	entry.Reference.Chain = myCore.ChainBase
	if _, err := NewCatalog([]PoolMetadata{entry}); err == nil {
		t.Fatal("NewCatalog() non-Solana error=nil")
	}
	entry = validPoolMetadata(t)
	entry.TokenB.Mint = entry.TokenA.Mint
	if _, err := NewCatalog([]PoolMetadata{entry}); err == nil {
		t.Fatal("NewCatalog() duplicate token error=nil")
	}
}

func validPoolMetadata(t *testing.T) PoolMetadata {
	t.Helper()
	pool := mustCatalogAddress(t, 1)
	return PoolMetadata{
		Reference: myCore.PoolReference{Chain: myCore.ChainSolana, Network: myCore.NetworkMainnet, Protocol: " Raydium-CLMM ", PoolID: pool.String()},
		Address:   pool,
		TokenA:    TokenMetadata{Symbol: " sol ", Mint: mustCatalogAddress(t, 2), Decimals: 9},
		TokenB:    TokenMetadata{Symbol: "USDC", Mint: mustCatalogAddress(t, 3), Decimals: 6},
		VaultA:    mustCatalogAddress(t, 4),
		VaultB:    mustCatalogAddress(t, 5),
		Program:   ProgramMetadata{ProgramID: mustCatalogAddress(t, 6)},
	}
}

func mustCatalogAddress(t *testing.T, suffix byte) Address {
	t.Helper()
	var address Address
	address[len(address)-1] = suffix
	return address
}
