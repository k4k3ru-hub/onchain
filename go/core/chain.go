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

