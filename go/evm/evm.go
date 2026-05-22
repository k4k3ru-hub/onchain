//
// evm.go
//
package evm

import (
    "fmt"
    "strings"

    "github.com/ethereum/go-ethereum/common"
)


//
// Parse hex string to EVM address.
//
// Version:
//   - 2026-05-22: Added.
//
func ParseAddress(address string) (common.Address, error) {
    s := strings.TrimSpace(address)
    if s == "" {
        return common.Address{}, fmt.Errorf("missing required parameter: address=%q", "empty")
    }
    if len(s) > 42 {
        return common.Address{}, fmt.Errorf("invalid parameter: address=%q", "too long")
    }
    if !common.IsHexAddress(s) {
        return common.Address{}, fmt.Errorf("invalid parameter: address=%q", s)
    }

    return common.HexToAddress(s), nil
}
