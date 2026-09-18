package core

import "testing"

// TestConfirmationPolicyDefaults verifies NewPair confirmation behavior.
//
// Version:
//   - 2026-09-18: Added.
func TestConfirmationPolicyDefaults(t *testing.T) {
	for _, d := range buildDefaultDepositPolicies() {
		p, err := ResolveConfirmationPolicy(d.Chain, d.Network)
		if err != nil || p.RequiredConfirmations != d.RequiredConfirmations {
			t.Fatalf("policy %s/%s: %+v %v", d.Chain, d.Network, p, err)
		}
	}
	p, err := ResolveConfirmationPolicy(ChainRobinhood, NetworkMainnet)
	if err != nil || p.RequiredConfirmations != 12 {
		t.Fatal(p, err)
	}
	if _, err = ResolveConfirmationPolicy(Chain("missing"), NetworkMainnet); err == nil {
		t.Fatal("unknown policy accepted")
	}
}

// TestConfirmationDepth verifies NewPair confirmation behavior.
//
// Version:
//   - 2026-09-18: Added.
func TestConfirmationDepth(t *testing.T) {
	p := ConfirmationPolicy{RequiredConfirmations: 12}
	for _, tt := range []struct {
		head, event uint64
		want        bool
	}{{110, 100, false}, {111, 100, true}, {99, 100, false}, {^uint64(0), 0, true}} {
		if p.IsConfirmed(tt.head, tt.event) != tt.want {
			t.Fatal(tt)
		}
	}
	if (ConfirmationPolicy{}).IsConfirmed(100, 1) {
		t.Fatal("zero threshold accepted")
	}
}
