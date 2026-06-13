//
// chain.go
//
package core

import (
    "fmt"
)


type Chain string

const (
    ChainEthereum  Chain = "ethereum"
    ChainBase      Chain = "base"
    ChainBNB       Chain = "bnb"
    ChainPolygon   Chain = "polygon"
    ChainAvalanche Chain = "avalanche"
    ChainSolana    Chain = "solana"
    ChainSui       Chain = "sui"
)


//
// Check whether chain is valid.
//
// Version:
//   - 2026-05-17: Added.
//
func (c Chain) IsValid() bool {
    switch c {
    case ChainEthereum,
        ChainBase,
        ChainBNB,
        ChainPolygon,
        ChainAvalanche,
        ChainSolana,
        ChainSui:
        return true
    default:
        return false
    }
}


//
// Validate chain.
//
// Version:
//   - 2026-05-17: Added.
//
func (c Chain) Validate() error {
    if string(c) == "" { 
        return fmt.Errorf("missing required parameter: chain=%q", "empty") 
    }
    if len(c) > 16 {
        return fmt.Errorf("invalid parameter: chain=%q", "too long")
    }
    if !c.IsValid() {
        return fmt.Errorf("invalid parameter: chain=%q", c)
    }
    return nil
}


//
// Convert chain to string.
//
// Version:
//   - 2026-05-17: Added.
//
func (c Chain) String() string {
    return string(c)
}


//
// Resolve chain family.
//
func (c Chain) ResolveChainFamily() (ChainFamily, error) {
    // Guard.
    if err := c.Validate(); err != nil {
        return "", fmt.Errorf("failed to resolve chain family: %w", err)
    }

    switch c {
    case ChainEthereum, ChainBase, ChainBNB, ChainPolygon, ChainAvalanche:
        return ChainFamilyEVM, nil
    case ChainSolana:
        return ChainFamilySolana, nil
    case ChainSui:
        return ChainFamilySui, nil
    default:
        return "", fmt.Errorf("failed to resolve chain family: invalid parameter: chain=%q", string(c))
    }
}


type ChainFamily string

const (
    ChainFamilyEVM    ChainFamily = "evm"
    ChainFamilySolana ChainFamily = "solana"
    ChainFamilySui    ChainFamily = "sui"
)


//
// Check whether chain family is valid.
//
// Version:
//   - 2026-06-12: Added.
//
func (f ChainFamily) IsValid() bool {
    switch f {
    case ChainFamilyEVM,
        ChainFamilySolana,
        ChainFamilySui:
        return true
    default:
        return false
    }
}


//
// Validate chain family.
//
// Version:
//   - 2026-06-12: Added.
//
func (f ChainFamily) Validate() error {
    if string(f) == "" {
        return fmt.Errorf("missing required parameter: chain_family=%q", "empty")
    }
    if len(f) > 16 {
        return fmt.Errorf("invalid parameter: chain_family=%q", "too long")
    }
    if !f.IsValid() {
        return fmt.Errorf("invalid parameter: chain_family=%q", f)
    }
    return nil
}


//
// Convert chain family to string.
//
// Version:
//   - 2026-06-172 Added.
//
func (f ChainFamily) String() string {
    return string(f)
}


