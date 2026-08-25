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

func TestDefaultPolygonUSDCAssets(t *testing.T) {
	registry, err := NewAssetRegistry().WithDefaultAssets()
	if err != nil {
		t.Fatalf("WithDefaultAssets() error = %v", err)
	}

	tests := []struct {
		network      Network
		wantTokenRef string
	}{
		{network: NetworkMainnet, wantTokenRef: "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359"},
		{network: NetworkAmoy, wantTokenRef: "0x41E94Eb019C0762f9Bfcf9Fb1E58725BfB0e7582"},
	}
	for _, test := range tests {
		asset, err := registry.Get(ChainPolygon, test.network, TokenUSDC)
		if err != nil {
			t.Fatalf("Get(%q, %q, %q) error = %v", ChainPolygon, test.network, TokenUSDC, err)
		}
		if asset.IsNative || asset.Decimals != 6 || asset.Name != "USD Coin" || asset.TokenRef == nil || !strings.EqualFold(*asset.TokenRef, test.wantTokenRef) {
			t.Errorf("Polygon %q USDC asset = %+v, want ERC-20 USD Coin with 6 decimals and token reference %q", test.network, asset, test.wantTokenRef)
		}
	}
}

func TestDefaultPolygonDepositPolicies(t *testing.T) {
	registry, err := NewDepositPolicyRegistry().WithDefaultDepositPolicies()
	if err != nil {
		t.Fatalf("WithDefaultDepositPolicies() error = %v", err)
	}

	for _, network := range []Network{NetworkMainnet, NetworkAmoy} {
		for _, token := range []Token{TokenPOL, TokenUSDC, TokenJPYC} {
			policy, err := registry.Get(ChainPolygon, network, token)
			if err != nil {
				t.Fatalf("Get(%q, %q, %q) error = %v", ChainPolygon, network, token, err)
			}
			if policy.RequiredConfirmations != 12 {
				t.Errorf("Polygon %q %q required confirmations = %d, want 12", network, token, policy.RequiredConfirmations)
			}
		}
	}
}
