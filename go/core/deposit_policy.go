//
// deposit_policy.go
//
package core

import (
    "fmt"
    "sync"    
)


var (
    defaultDepositPolicyRegistry = NewDepositPolicyRegistry()
)


//
// Deposit policy.
//
type DepositPolicy struct {
    Chain                 Chain
    Network               Network
    Token                 Token
    RequiredConfirmations uint64
}

type DepositPolicyKey struct {
    Chain   Chain
    Network Network
    Token   Token
}

type DepositPolicyRegistry struct {
    mu                 sync.RWMutex
    byDepositPolicyKey map[DepositPolicyKey]*DepositPolicy
}


//
// Get default deposit policy registry.
//
// Version:
//   - 2026-05-28: Added.
//
func DefaultDepositPolicyRegistry() *DepositPolicyRegistry {
    return defaultDepositPolicyRegistry
}


//
// Create deposit policy.
//
// Version:
//   - 2026-05-28: Added.
//
func NewDepositPolicy(c Chain, n Network, t Token, requiredConfirmations uint64) *DepositPolicy {
    return &DepositPolicy{
        Chain:                 c,
        Network:               n,
        Token:                 t,
        RequiredConfirmations: requiredConfirmations,
    }
}


//
// Create deposit policy registry.
//
// Version:
//   - 2026-05-28: Added.
//
func NewDepositPolicyRegistry() *DepositPolicyRegistry {
    return &DepositPolicyRegistry{
        byDepositPolicyKey: make(map[DepositPolicyKey]*DepositPolicy),
    }
}


//
// Check whether tx is confirmed.
//
// Version:
//   - 2026-05-28: Added.
//
func (p *DepositPolicy) IsConfirmed(latestBlockNumber, txBlockNumber uint64) bool {
    // Guard.
    if p == nil {
        return false
    }
    if p.RequiredConfirmations == 0 {
        return false
    }
    if latestBlockNumber < txBlockNumber {
        return false
    }

    return latestBlockNumber-txBlockNumber+1 >= p.RequiredConfirmations
}


//
// Build deposit policy key.
//
// Version:
//   - 2026-05-28: Added.
//
func (p *DepositPolicy) Key() DepositPolicyKey {
    if p == nil {
        return DepositPolicyKey{}
    }
    return DepositPolicyKey{
        Chain:   p.Chain,
        Network: p.Network,
        Token:   p.Token,
    }
}


//
// Validate deposit policy.
//
// Version:
//   - 2026-05-28: Added.
//
func (p *DepositPolicy) Validate() error {
    if p == nil {
        return fmt.Errorf("missing required parameter: deposit_policy=null")
    }

    if err := p.Chain.Validate(); err != nil {
        return err
    }

    if err := p.Network.Validate(); err != nil {
        return err
    }

    if err := p.Token.Validate(); err != nil {
        return err
    }

    if p.RequiredConfirmations == 0 {
        return fmt.Errorf("invalid parameter: required_confirmations=0")
    }

    return nil
}


//
// Get deposit policy by key. 
//
// Version:
//   - 2026-06-12: Added.
//
func (r *DepositPolicyRegistry) Get(key DepositPolicyKey) (*DepositPolicy, error) {
    if r == nil {
        return nil, fmt.Errorf("failed to get deposit policy: missing required parameter: deposit_policy_registry=null")
    }

    r.mu.RLock()
    defer r.mu.RUnlock()

    policy, ok := r.byDepositPolicyKey[key]
    if !ok {
        return nil, fmt.Errorf("failed to get deposit policy: deposit_policy=%q chain=%q network=%q token=%q", "not found", key.Chain, key.Network, key.Token)
    }

    cp := *policy
    return &cp, nil
}


//
// Register deposit policy.
//
// Version:
//   - 2026-06-12: Added.
//
func (r *DepositPolicyRegistry) Register(policy *DepositPolicy) error {
    if r == nil {
        return fmt.Errorf("failed to register deposit policy: missing required parameter: deposit_policy_registry=null")
    }
    if err := policy.Validate(); err != nil {
        return fmt.Errorf("failed to register deposit policy: %w", err)
    }

    cp := *policy
    key := cp.Key()

    r.mu.Lock()
    defer r.mu.Unlock()

    r.byDepositPolicyKey[key] = &cp

    return nil
}


//
// Register all deposit policies.
//
// Version:
//   - 2026-06-12: Added.
//
func (r *DepositPolicyRegistry) RegisterAll(depositPolicies ...*DepositPolicy) error {
    if r == nil {
        return fmt.Errorf("failed to register deposit policies: missing required parameter: deposit_policy_registry=null")
    }

    for _, policy := range depositPolicies {
        if err := r.Register(policy); err != nil {
            return err
        }
    }

    return nil
}


//
// Add default deposit policies.
//
// Version:
//   - 2026-06-12: Added.
//
func (r *DepositPolicyRegistry) WithDefaultDepositPolicies() (*DepositPolicyRegistry, error) {
    // Guard.
    if r == nil {
        return nil, fmt.Errorf("failed to register default deposit policies: missing required parameter: deposit_policy_registry=null")
    }

    if err := r.RegisterAll(buildDefaultDepositPolicies()...); err != nil {
        return nil, err
    }

    return r, nil
}


//
// Build default deposit policies.
//
// Version:
//   - 2026-06-12: Added.
//
func buildDefaultDepositPolicies() []*DepositPolicy {
    return []*DepositPolicy{
        // Ethereum: Mainnet.
        NewDepositPolicy(ChainEthereum, NetworkMainnet, TokenETH, 12),
        NewDepositPolicy(ChainEthereum, NetworkMainnet, TokenUSDC, 12),

        // Ethereum: Sepolia.
        NewDepositPolicy(ChainEthereum, NetworkSepolia, TokenETH, 12),
        NewDepositPolicy(ChainEthereum, NetworkSepolia, TokenUSDC, 12),

        // Base: Mainnet.
        NewDepositPolicy(ChainBase, NetworkMainnet, TokenETH, 12),
        NewDepositPolicy(ChainBase, NetworkMainnet, TokenUSDC, 12),

        // Base: Sepolia.
        NewDepositPolicy(ChainBase, NetworkSepolia, TokenETH, 12),
        NewDepositPolicy(ChainBase, NetworkSepolia, TokenUSDC, 12),

        // BNB: Mainnet.
        NewDepositPolicy(ChainBNB, NetworkMainnet, TokenBNB, 15),

        // Polygon.
        NewDepositPolicy(ChainPolygon, NetworkMainnet, TokenPOL, 128),

        // Avalanche.
        NewDepositPolicy(ChainAvalanche, NetworkMainnet, TokenAVAX, 20),

        // Solana.
        NewDepositPolicy(ChainSolana, NetworkMainnet, TokenSOL, 32),

        // Sui.
        NewDepositPolicy(ChainSui, NetworkMainnet, TokenSUI, 1),
    }
}
