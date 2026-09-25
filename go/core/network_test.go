package core

import (
	"strings"
	"testing"
)

// TestNetworkValidation verifies open network names and bounded, unambiguous identifiers.
//
// Version:
//   - 2026-09-25: Added.
func TestNetworkValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		network   Network
		wantError string
	}{
		{name: "mainnet", network: NetworkMainnet},
		{name: "testnet", network: NetworkTestnet},
		{name: "devnet", network: NetworkDevnet},
		{name: "sepolia", network: NetworkSepolia},
		{name: "holesky", network: NetworkHolesky},
		{name: "amoy", network: NetworkAmoy},
		{name: "custom", network: "custom-testnet"},
		{name: "local", network: "localnet"},
		{name: "case preserved", network: "MAINNET"},
		{name: "maximum length", network: Network(strings.Repeat("a", 16))},
		{name: "empty", wantError: "network=empty"},
		{name: "too long", network: Network(strings.Repeat("a", 17)), wantError: "network=too_long actual_length=17 max_length=16"},
		{name: "leading space", network: " mainnet", wantError: "network=invalid"},
		{name: "trailing space", network: "mainnet ", wantError: "network=invalid"},
		{name: "internal space", network: "custom net", wantError: "network=invalid"},
		{name: "tab", network: "main\tnet", wantError: "network=invalid"},
		{name: "newline", network: "mainnet\n", wantError: "network=invalid"},
		{name: "null byte", network: "main\x00net", wantError: "network=invalid"},
		{name: "unicode space", network: "main\u00a0net", wantError: "network=invalid"},
		{name: "unicode control", network: "main\u009fnet", wantError: "network=invalid"},
		{name: "invalid utf8", network: Network("main\xffnet"), wantError: "network=invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.network
			err := test.network.Validate()
			if test.wantError == "" {
				if err != nil || !test.network.IsValid() {
					t.Fatalf("valid network rejected: %v", err)
				}
			} else if err == nil || err.Error() != "failed to validate network: "+test.wantError || test.network.IsValid() {
				t.Fatalf("invalid network: error=%v want=%q", err, test.wantError)
			}
			if test.network != before {
				t.Fatal("validation changed the network identifier")
			}
		})
	}
}

// TestCustomNetworkReferenceRequiresConfiguredPolicy separates identity from service support.
//
// Version:
//   - 2026-09-25: Added.
func TestCustomNetworkReferenceRequiresConfiguredPolicy(t *testing.T) {
	reference := PoolReference{Chain: ChainBase, Network: "custom-testnet", Protocol: "uniswap-v3", PoolID: "pool"}
	if err := reference.Validate(); err != nil {
		t.Fatalf("custom network reference rejected: %v", err)
	}
	if _, err := ResolveConfirmationPolicy(reference.Chain, reference.Network); err == nil {
		t.Fatal("unconfigured network received a confirmation policy")
	}
}
