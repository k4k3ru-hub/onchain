//
// symbol.go
//
package onchain

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
    if !s.IsValid() {
        return fmt.Errorf("invalid parameter: symbol=%q", truncateRunes(string(s), 16))
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
