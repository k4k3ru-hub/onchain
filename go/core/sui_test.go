package core

import "testing"

func TestDefaultSuiAssets(t *testing.T) {
	registry, err := NewAssetRegistry().WithDefaultAssets()
	if err != nil {
		t.Fatalf("WithDefaultAssets() returned an unexpected error: %v", err)
	}
	for _, network := range []Network{NetworkMainnet, NetworkTestnet, NetworkDevnet} {
		asset, err := registry.Get(ChainSui, network, TokenSUI)
		if err != nil {
			t.Fatalf("Get() network=%q returned an unexpected error: %v", network, err)
		}
		if asset.Decimals != 9 || !asset.IsNative {
			t.Fatalf("Get() network=%q asset=%+v, want native SUI with 9 decimals", network, asset)
		}
	}
}
