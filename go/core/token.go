//
// token.go
//
package core

import (
    "fmt"
)


type Token string

const (
    TokenAVAX Token = "AVAX"
    TokenBNB  Token = "BNB"
    TokenETH  Token = "ETH"
    TokenPOL  Token = "POL"
    TokenSOL  Token = "SOL"
    TokenSUI  Token = "SUI"
    TokenUSDC Token = "USDC"
    TokenUSDT Token = "USDT"
)


//
// Check whether token is valid.
//
// Version:
//   - 2026-05-17: Added.
//
func (t Token) IsValid() bool {
    switch t {
    case TokenAVAX,
        TokenBNB,
        TokenETH,
        TokenPOL,
        TokenSOL,
        TokenSUI,
        TokenUSDC,
        TokenUSDT:
        return true
    default:
        return false
    }
}


//
// Validate token.
//
// Version:
//   - 2026-05-17: Added.
//
func (t Token) Validate() error {
    if string(t) == "" {
        return fmt.Errorf("missing required parameter: token=%q", "empty")
    }
    if len(t) > 16 {
        return fmt.Errorf("invalid parameter: token=%q", "too long")
    }
    if !t.IsValid() {
        return fmt.Errorf("invalid parameter: token=%q", string(t))
    }
    return nil
}


//
// Convert token to string.
//
// Version:
//   - 2026-05-17: Added.
//
func (t Token) String() string {
    return string(t)
}
