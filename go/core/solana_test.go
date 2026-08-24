package core

import "testing"

func TestDefaultSolanaAssets(t *testing.T) {
	registry, err := NewAssetRegistry().WithDefaultAssets()
	if err != nil {
		t.Fatalf("WithDefaultAssets() error = %v", err)
	}

	for _, network := range []Network{NetworkMainnet, NetworkDevnet} {
		asset, err := registry.Get(ChainSolana, network, TokenSOL)
		if err != nil {
			t.Fatalf("Get(%q, %q, %q) error = %v", ChainSolana, network, TokenSOL, err)
		}
		if !asset.IsNative || asset.Decimals != 9 || asset.TokenRef != nil {
			t.Errorf("Solana %q SOL asset = %+v, want native asset with 9 decimals", network, asset)
		}
	}
}
