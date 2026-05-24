//
// asset.go
//
package core

import (
    "fmt"
    "strings"
    "unicode/utf8"
)


//
// Asset.
//
// Parameters:
//   - AssetID:
//     - EVM: 0xA0b8...
//     - Solana: EPjFWd...
//     - Sui: 0x2::sui::SUI
//
type Asset struct {
    Chain        Chain
    Network      Network
    Symbol       Symbol
    Decimals     uint8
    Name         string
    IsNative     bool
    AssetID      *string
}


type AssetKey struct {
    Chain   Chain
    Network Network
    Symbol  Symbol
}


//
// Create new asset.
//
// Version:
//   - 2026-05-17: Added.
//
func NewAsset(c Chain, n Network, s Symbol, decimals uint8, name string, isNative bool) *Asset {
    return &Asset{
        Chain:    c,
        Network:  n,
        Symbol:   s,
        Decimals: decimals,
        Name:     name,
        IsNative: isNative,
    }
}


//
// Set token address.
//
// Version:
//   - 2026-05-17: Added.
//
func (a *Asset) WithAssetID(tokenAddress string) *Asset {
    if a == nil {
        return nil
    }
    a.AssetID = &tokenAddress
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
        Symbol:  a.Symbol,
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

    if err := a.Symbol.Validate(); err != nil {
        return err
    }

    if a.Decimals > 77 {
        return fmt.Errorf("invalid parameter: decimals=%d", a.Decimals)
    }

    if strings.TrimSpace(a.Name) == "" {
        return fmt.Errorf("missing required parameter: name=%q", "empty")
    }
    if utf8.RuneCountInString(strings.TrimSpace(a.Name)) > 64 {
        return fmt.Errorf("invalid parameter: max_length=64 name=%q", "too long")
    }

    if a.IsNative {
        if a.AssetID != nil {
            return fmt.Errorf("invalid parameter: native asset must not have token_address")
        }
        return nil
    }

    if a.AssetID == nil {
        return fmt.Errorf("missing required parameter: token_address=%q", "empty")
    }

    tokenAddress := strings.TrimSpace(*a.AssetID)
    if tokenAddress == "" {
        return fmt.Errorf("missing required parameter: token_address=%q", "empty")
    }
    if utf8.RuneCountInString(tokenAddress) > 255 {
        return fmt.Errorf("invalid parameter: max_length=255 token_address=%q", "too long")
    }

    return nil
}

