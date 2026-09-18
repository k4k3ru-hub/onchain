package core

import "fmt"

// ConfirmationPolicy defines the service's required observation depth.
// It does not equate observation depth with consensus finality.
type ConfirmationPolicy struct {
	Chain                 Chain
	Network               Network
	RequiredConfirmations uint64
}

// ResolveConfirmationPolicy returns the network's service confirmation policy.
// Deposit policies remain independent; their existing thresholds are reused here.
//
// Version:
//   - 2026-09-18: Added.
func ResolveConfirmationPolicy(chain Chain, network Network) (ConfirmationPolicy, error) {
	p := ConfirmationPolicy{Chain: chain, Network: network}
	for _, deposit := range buildDefaultDepositPolicies() {
		if deposit.Chain != chain || deposit.Network != network {
			continue
		}
		if p.RequiredConfirmations != 0 && p.RequiredConfirmations != deposit.RequiredConfirmations {
			return p, fmt.Errorf("failed to resolve confirmation policy: token thresholds differ: chain=%q network=%q", chain, network)
		}
		p.RequiredConfirmations = deposit.RequiredConfirmations
	}
	if chain == ChainRobinhood && network == NetworkMainnet {
		p.RequiredConfirmations = 12
	}
	if p.RequiredConfirmations == 0 {
		return p, fmt.Errorf("failed to resolve confirmation policy: policy=invalid chain=%q network=%q", chain, network)
	}
	return p, nil
}

// IsConfirmed checks EVM observation depth, counting the event block as one.
// Non-EVM adapters must use their native confirmation semantics.
//
// Version:
//   - 2026-09-18: Added.
func (p ConfirmationPolicy) IsConfirmed(latest, position uint64) bool {
	return p.RequiredConfirmations > 0 && latest >= position && latest-position >= p.RequiredConfirmations-1
}
