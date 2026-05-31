//
// asset.go
//
package core

import (
    "fmt"
    "strings"
    "sync"
    "unicode/utf8"
)


var (
    defaultAssetRegistry = NewAssetRegistry()
)


//
// Asset.
//
// Parameters:
//   - TokenRef:
//     - EVM: 0xA0b8...
//     - Solana: EPjFWd...
//     - Sui: 0x2::sui::SUI
//
type Asset struct {
    Chain    Chain
    Network  Network
    Token    Token
    Decimals uint8
    Name     string
    IsNative bool
    TokenRef *string
}


type AssetKey struct {
    Chain   Chain
    Network Network
    Token   Token
}


type AssetRegistry struct {
    mu         sync.RWMutex
    byAssetKey map[AssetKey]*Asset
}


//
// Get default asset registry.
//
// Version:
//   - 2026-05-17: Added.
//
func DefaultAssetRegistry() *AssetRegistry {
    return defaultAssetRegistry
}


//
// Create asset.
//
// Version:
//   - 2026-05-17: Added.
//
func NewAsset(c Chain, n Network, t Token, decimals uint8, name string, isNative bool) *Asset {
    return &Asset{
        Chain:    c,
        Network:  n,
        Token:    t,
        Decimals: decimals,
        Name:     name,
        IsNative: isNative,
    }
}


//
// Create asset registry.
//
// Version:
//   - 2026-05-17: Added.
//
func NewAssetRegistry() *AssetRegistry {
    return &AssetRegistry{
        byAssetKey: make(map[AssetKey]*Asset),
    }
}


//
// Set token reference such as contract address.
//
// Version:
//   - 2026-05-17: Added.
//
func (a *Asset) WithTokenRef(ref string) *Asset {
    if a == nil {
        return nil
    }
    a.TokenRef = &ref
    return a
}


//
// Build asset key.
//
// Version:
//   - 2026-05-17: Added.
//
func (a *Asset) Key() AssetKey {
    if a == nil {
        return AssetKey{}
    }
    return AssetKey{
        Chain:   a.Chain,
        Network: a.Network,
        Token:   a.Token,
    }
}


//
// Validate asset.
//
// Version:
//   - 2026-05-17: Added.
//
func (a *Asset) Validate() error {
    if a == nil {
        return fmt.Errorf("invalid parameter: asset=null")
    }

    if err := a.Chain.Validate(); err != nil {
        return err
    }

    if err := a.Network.Validate(); err != nil {
        return err
    }

    if err := a.Token.Validate(); err != nil {
        return err
    }

    if a.Decimals > 77 {
        return fmt.Errorf("invalid parameter: max_decimals=77 decimals=%d", a.Decimals)
    }

    if strings.TrimSpace(a.Name) == "" {
        return fmt.Errorf("missing required parameter: name=%q", "empty")
    }
    if utf8.RuneCountInString(strings.TrimSpace(a.Name)) > 64 {
        return fmt.Errorf("invalid parameter: max_length=64 name=%q", "too long")
    }

    if a.IsNative {
        if a.TokenRef != nil {
            return fmt.Errorf("invalid parameter: native asset must not have token_address")
        }
        return nil
    }

    if a.TokenRef == nil {
        return fmt.Errorf("missing required parameter: token_address=%q", "empty")
    }

    tokenAddress := strings.TrimSpace(*a.TokenRef)
    if tokenAddress == "" {
        return fmt.Errorf("missing required parameter: token_address=%q", "empty")
    }
    if utf8.RuneCountInString(tokenAddress) > 255 {
        return fmt.Errorf("invalid parameter: max_length=255 token_address=%q", "too long")
    }

    return nil
}


func (r *AssetRegistry) RegisterDefaultAssets() error {
    // Ethereum.
    ethereumMainETH := NewAsset(ChainEthereum, NetworkMainnet, TokenETH, 18, "Ether", true)
    if err := r.Register(ethereumMainETH); err != nil {
        return err
    }

    ethereumMainUSDC := NewAsset(ChainEthereum, NetworkMainnet, TokenUSDC, 6, "USD Coin", false).WithTokenRef("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
    if err := r.Register(ethereumMainUSDC); err != nil {
        return err
    }


    // Base.
    baseMainETH := NewAsset(ChainBase, NetworkMainnet, TokenETH, 18, "Ether", true)
    if err := r.Register(baseMainETH); err != nil {
        return err
    }

    baseMainUSDC := NewAsset(ChainBase, NetworkMainnet, TokenUSDC, 6, "USD Coin", false).WithTokenRef("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")
    if err := r.Register(baseMainUSDC); err != nil {
        return err
    }


    // BNB.
    bnbMainBNB := NewAsset(ChainBNB, NetworkMainnet, TokenBNB, 18, "BNB", true)
    if err := r.Register(bnbMainBNB); err != nil {
        return err
    }


    // Polygon.
    polygonMainPOL := NewAsset(ChainPolygon, NetworkMainnet, TokenPOL, 18, "POL", true)
    if err := r.Register(polygonMainPOL); err != nil {
        return err
    }

    // Avalanche.
    avalancheMainAVAX := NewAsset(ChainAvalanche, NetworkMainnet, TokenAVAX, 18, "Avalanche", true)
    if err := r.Register(avalancheMainAVAX); err != nil {
        return err
    }


    // Solana.
    solanaMainSOL := NewAsset(ChainSolana, NetworkMainnet, TokenSOL, 9, "Solana", true)
    if err := r.Register(solanaMainSOL); err != nil {
        return err
    }


    // Sui.
    suiMainSUI := NewAsset(ChainSui, NetworkMainnet, TokenSUI, 9, "Sui", true)
    if err := r.Register(suiMainSUI); err != nil {
        return err
    }


    return nil
}


func (r *AssetRegistry) Register(asset *Asset) error {
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


func (r *AssetRegistry) Get(key AssetKey) *Asset {
    if r == nil {
        return nil
    }

    r.mu.RLock()
    defer r.mu.RUnlock()

    asset, ok := r.byAssetKey[key]
    if !ok {
        return nil
    }

    cp := *asset
    return &cp
}
