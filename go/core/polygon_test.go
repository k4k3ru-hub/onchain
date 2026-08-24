package core

import (
	"strings"
	"testing"
)

func TestPolygonNetworks(t *testing.T) {
	for _, network := range []Network{NetworkMainnet, NetworkAmoy} {
		if err := network.Validate(); err != nil {
			t.Errorf("Network(%q).Validate() error = %v", network, err)
		}
	}
}

func TestDefaultPolygonAssets(t *testing.T) {
	registry, err := NewAssetRegistry().WithDefaultAssets()
	if err != nil {
		t.Fatalf("WithDefaultAssets() error = %v", err)
	}

	for _, network := range []Network{NetworkMainnet, NetworkAmoy} {
		asset, err := registry.Get(ChainPolygon, network, TokenPOL)
		if err != nil {
			t.Fatalf("Get(%q, %q, %q) error = %v", ChainPolygon, network, TokenPOL, err)
		}
		if !asset.IsNative || asset.Decimals != 18 || asset.TokenRef != nil {
			t.Errorf("Polygon %q POL asset = %+v, want native asset with 18 decimals", network, asset)
		}
	}
}

func TestDefaultPolygonJPYCAssets(t *testing.T) {
	registry, err := NewAssetRegistry().WithDefaultAssets()
	if err != nil {
		t.Fatalf("WithDefaultAssets() error = %v", err)
	}

	const wantTokenRef = "0xE7C3D8C9a439feDe00D2600032D5dB0Be71C3c29"
	for _, network := range []Network{NetworkMainnet, NetworkAmoy} {
		asset, err := registry.Get(ChainPolygon, network, TokenJPYC)
		if err != nil {
			t.Fatalf("Get(%q, %q, %q) error = %v", ChainPolygon, network, TokenJPYC, err)
		}
		if asset.IsNative || asset.Decimals != 18 || asset.Name != "JPY Coin" || asset.TokenRef == nil || !strings.EqualFold(*asset.TokenRef, wantTokenRef) {
			t.Errorf("Polygon %q JPYC asset = %+v, want ERC-20 JPY Coin with 18 decimals and token reference %q", network, asset, wantTokenRef)
		}
	}
}

func TestDefaultPolygonDepositPolicies(t *testing.T) {
	registry, err := NewDepositPolicyRegistry().WithDefaultDepositPolicies()
	if err != nil {
		t.Fatalf("WithDefaultDepositPolicies() error = %v", err)
	}

	for _, network := range []Network{NetworkMainnet, NetworkAmoy} {
		policy, err := registry.Get(ChainPolygon, network, TokenPOL)
		if err != nil {
			t.Fatalf("Get(%q, %q, %q) error = %v", ChainPolygon, network, TokenPOL, err)
		}
		if policy.RequiredConfirmations != 128 {
			t.Errorf("Polygon %q required confirmations = %d, want 128", network, policy.RequiredConfirmations)
		}
	}
}
