//
// registry.go
//
package onchain

import (
    "fmt"
    "sync"
)


var (
    defaultRegistry = NewRegistry()
)


type Registry struct {
    mu         sync.RWMutex
    byAssetKey map[AssetKey]*Asset
}


//
// Create new registry.
//
// Version:
//   - 2026-05-17: Added.
//
func NewRegistry() *Registry {
    return &Registry{
        byAssetKey: make(map[AssetKey]*Asset),
    }
}


func DefaultRegistry() *Registry {
    return defaultRegistry
}


func (r *Registry) RegisterDefaultAssets() error {
    // Ethereum.
    ethereumMainETH := NewAsset(ChainEthereum, NetworkMainnet, SymbolETH, 18, "Ether", true)
    if err := r.Register(ethereumMainETH); err != nil {
        return err
    }

    ethereumMainUSDC := NewAsset(ChainEthereum, NetworkMainnet, SymbolUSDC, 6, "USD Coin", false).WithTokenAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
    if err := r.Register(ethereumMainUSDC); err != nil {
        return err
    }


    // Base.
    baseMainETH := NewAsset(ChainBase, NetworkMainnet, SymbolETH, 18, "Ether", true)
    if err := r.Register(baseMainETH); err != nil {
        return err
    }

    baseMainUSDC := NewAsset(ChainBase, NetworkMainnet, SymbolUSDC, 6, "USD Coin", false).WithTokenAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")
    if err := r.Register(baseMainUSDC); err != nil {
        return err
    }


    // BNB.
    bnbMainBNB := NewAsset(ChainBNB, NetworkMainnet, SymbolBNB, 18, "BNB", true)
    if err := r.Register(bnbMainBNB); err != nil {
        return err
    }


    // Polygon.
    polygonMainPOL := NewAsset(ChainPolygon, NetworkMainnet, SymbolPOL, 18, "POL", true)
    if err := r.Register(polygonMainPOL); err != nil {
        return err
    }

    // Avalanche.
    avalancheMainAVAX := NewAsset(ChainAvalanche, NetworkMainnet, SymbolAVAX, 18, "Avalanche", true)
    if err := r.Register(avalancheMainAVAX); err != nil {
        return err
    }


    // Solana.
    solanaMainSOL := NewAsset(ChainSolana, NetworkMainnet, SymbolSOL, 9, "Solana", true)
    if err := r.Register(solanaMainSOL); err != nil {
        return err
    }


    // Sui.
    suiMainSUI := NewAsset(ChainSui, NetworkMainnet, SymbolSUI, 9, "Sui", true)
    if err := r.Register(suiMainSUI); err != nil {
        return err
    }


    return nil
}


func (r *Registry) Register(asset *Asset) error {
    if r == nil {
        return fmt.Errorf("failed to register asset: missing required parameter: registry=null")
    }
    if err := asset.Validate(); err != nil {
        return fmt.Errorf("failed to register asset: %w", err)
    }

    cp := *asset
    key := cp.Key()

    r.mu.Lock()
    defer r.mu.Unlock()

    r.byAssetKey[key] = &cp

    return nil
}


func (r *Registry) Get(key AssetKey) (*Asset, bool) {
    if r == nil {
        return nil, false
    }

    r.mu.RLock()
    defer r.mu.RUnlock()

    asset, ok := r.byAssetKey[key]
    if !ok {
        return nil, false
    }

    cp := *asset
    return &cp, true
}
