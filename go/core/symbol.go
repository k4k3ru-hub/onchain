//
// symbol.go
//
package core

import (
    "fmt"
)


type Symbol string

const (
    SymbolAVAX Symbol = "AVAX"
    SymbolBNB  Symbol = "BNB"
    SymbolETH  Symbol = "ETH"
    SymbolPOL  Symbol = "POL"
    SymbolSOL  Symbol = "SOL"
    SymbolSUI  Symbol = "SUI"
    SymbolUSDC Symbol = "USDC"
    SymbolUSDT Symbol = "USDT"
)


//
// Check whether symbol is valid.
//
// Version:
//   - 2026-05-17: Added.
//
func (s Symbol) IsValid() bool {
    switch s {
    case SymbolAVAX,
        SymbolBNB,
        SymbolETH,
        SymbolPOL,
        SymbolSOL,
        SymbolSUI,
        SymbolUSDC,
        SymbolUSDT:
        return true
    default:
        return false
    }
}


//
// Validate symbol.
//
// Version:
//   - 2026-05-17: Added.
//
func (s Symbol) Validate() error {
    if string(s) == "" {
        return fmt.Errorf("missing required parameter: symbol=%q", "empty")
    }
    if len(s) > 16 {
        return fmt.Errorf("invalid parameter: symbol=%q", "too long")
    }
    if !s.IsValid() {
        return fmt.Errorf("invalid parameter: symbol=%q", s)
    }
    return nil
}


//
// Convert symbol to string.
//
// Version:
//   - 2026-05-17: Added.
//
func (s Symbol) String() string {
    return string(s)
}
