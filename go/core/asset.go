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


//
// Get asset.
//
// Version:
//   - 2026-06-12: Added.
//
func (r *AssetRegistry) Get(c Chain, n Network, t Token) (*Asset, error) {
    if r == nil {
        return nil, fmt.Errorf("failed to get asset: missing required parameter: asset_registry=null")
    }

    r.mu.RLock()
    defer r.mu.RUnlock()

    asset, ok := r.byAssetKey[AssetKey{
        Chain:   c,
        Network: n,
        Token:   t,
    }]
    if !ok {
        return nil, fmt.Errorf("failed to get asset: asset=%q chain=%q network=%q token=%q", "not found", c, n, t)
    }

    cp := *asset
    return &cp, nil
}


//
// Register asset.
//
// Version:
//   - 2026-06-12: Added.
//
func (r *AssetRegistry) Register(asset *Asset) error {
    if r == nil {
        return fmt.Errorf("failed to register asset: missing required parameter: asset_registry=null")
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


//
// Register all assets.
//
// Version:
//   - 2026-06-12: Added.
//
func (r *AssetRegistry) RegisterAll(assets ...*Asset) error {
	if r == nil {
		return fmt.Errorf("failed to register assets: missing required parameter: asset_registry=null")
	}

	for _, asset := range assets {
		if err := r.Register(asset); err != nil {
			return err
		}
	}

	return nil
}


//
// Add default assets to asset registry.
//
// Version:
//   - 2026-06-12: Added.
//
func (r *AssetRegistry) WithDefaultAssets() (*AssetRegistry, error) {
    // Guard.
    if r == nil {
        return nil, fmt.Errorf("failed to register default assets: missing required parameter: asset_registry=null")
    }

    if err := r.RegisterAll(buildDefaultAssets()...); err != nil {
        return nil, err
    }

    return r, nil
}


//
// Build default assets.
//
// Version:
//   - 2026-06-12: Added.
//
func buildDefaultAssets() []*Asset {
    return []*Asset{
        // Ethereum: Mainnet.
        NewAsset(ChainEthereum, NetworkMainnet, TokenETH, 18, "Ether", true),
        NewAsset(ChainEthereum, NetworkMainnet, TokenUSDC, 6, "USD Coin", false).WithTokenRef("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"),

        // Ethereum: Sepolia.
        NewAsset(ChainEthereum, NetworkSepolia, TokenETH, 18, "Ether", true),
        NewAsset(ChainEthereum, NetworkSepolia, TokenUSDC, 6, "USD Coin", false).WithTokenRef("0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"),

        // Base: Mainnet.
        NewAsset(ChainBase, NetworkMainnet, TokenETH, 18, "Ether", true),
        NewAsset(ChainBase, NetworkMainnet, TokenUSDC, 6, "USD Coin", false).WithTokenRef("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"),

        // Base: Sepolia.
        NewAsset(ChainBase, NetworkSepolia, TokenETH, 18, "Ether", true),
        NewAsset(ChainBase, NetworkSepolia, TokenUSDC, 6, "USD Coin", false).WithTokenRef("0x036CbD53842c5426634e7929541eC2318f3dCF7e"),

        // BNB: Mainnet.
        NewAsset(ChainBNB, NetworkMainnet, TokenBNB, 18, "BNB", true),

        // Polygon: Mainnet.
        NewAsset(ChainPolygon, NetworkMainnet, TokenPOL, 18, "POL", true),

        // Avalanche: Mainnet.
        NewAsset(ChainAvalanche, NetworkMainnet, TokenAVAX, 18, "Avalanche", true),

        // Solana: Mainnet.
        NewAsset(ChainSolana, NetworkMainnet, TokenSOL, 9, "Solana", true),

        // Sui: Mainnet.
        NewAsset(ChainSui, NetworkMainnet, TokenSUI, 9, "Sui", true),
    }
}
