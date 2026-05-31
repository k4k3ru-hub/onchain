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


func (r *DepositPolicyRegistry) RegisterDefaultDepositPolicies() error {
    // Ethereum.
    ethereumMainETH := NewDepositPolicy(ChainEthereum, NetworkMainnet, TokenETH, 12)
    if err := r.Register(ethereumMainETH); err != nil {
        return err
    }

    ethereumMainUSDC := NewDepositPolicy(ChainEthereum, NetworkMainnet, TokenUSDC, 12)
    if err := r.Register(ethereumMainUSDC); err != nil {
        return err
    }

    // Base.
    baseMainETH := NewDepositPolicy(ChainBase, NetworkMainnet, TokenETH, 12)
    if err := r.Register(baseMainETH); err != nil {
        return err
    }

    baseMainUSDC := NewDepositPolicy(ChainBase, NetworkMainnet, TokenUSDC, 12)
    if err := r.Register(baseMainUSDC); err != nil {
        return err
    }

    // BNB.
    bnbMainBNB := NewDepositPolicy(ChainBNB, NetworkMainnet, TokenBNB, 15)
    if err := r.Register(bnbMainBNB); err != nil {
        return err
    }

    // Polygon.
    polygonMainPOL := NewDepositPolicy(ChainPolygon, NetworkMainnet, TokenPOL, 128)
    if err := r.Register(polygonMainPOL); err != nil {
        return err
    }

    // Avalanche.
    avalancheMainAVAX := NewDepositPolicy(ChainAvalanche, NetworkMainnet, TokenAVAX, 20)
    if err := r.Register(avalancheMainAVAX); err != nil {
        return err
    }

    // Solana.
    solanaMainSOL := NewDepositPolicy(ChainSolana, NetworkMainnet, TokenSOL, 32)
    if err := r.Register(solanaMainSOL); err != nil {
        return err
    }

    // Sui.
    suiMainSUI := NewDepositPolicy(ChainSui, NetworkMainnet, TokenSUI, 1)
    if err := r.Register(suiMainSUI); err != nil {
        return err
    }

    return nil
}


func (r *DepositPolicyRegistry) Register(policy *DepositPolicy) error {
    if r == nil {
        return fmt.Errorf("failed to register deposit policy: missing required parameter: registry=null")
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

func (r *DepositPolicyRegistry) Get(key DepositPolicyKey) *DepositPolicy {
    if r == nil {
        return nil
    }

    r.mu.RLock()
    defer r.mu.RUnlock()

    policy, ok := r.byDepositPolicyKey[key]
    if !ok {
        return nil
    }

    cp := *policy
    return &cp
}
