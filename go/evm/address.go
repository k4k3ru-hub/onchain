//
// address.go
//
package evm

import (
    "crypto/ecdsa"
    "fmt"
    "strings"

    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/crypto"
)


//
// Generate EVM address key pair.
//
// Return:
//   - EVM address: 0x...
//   - Private key hex: ... (without 0x prefix)
//
// Version:
//   - 2026-05-24: Added.
//
func GenerateAddressKeyPair() (string, string, error) {
    privateKey, err := crypto.GenerateKey()
    if err != nil {
        return "", "", fmt.Errorf("failed to generate evm address key pair: %w", err)
    }

    privateKeyBytes := crypto.FromECDSA(privateKey)
    privateKeyHex := fmt.Sprintf("%x", privateKeyBytes)

    publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
    if !ok {
        return "", "", fmt.Errorf("failed to generate evm address key pair: invalid parameter: public_key")
    }

    address := crypto.PubkeyToAddress(*publicKey)

    return address.Hex(), privateKeyHex, nil
}


//
// Normalize EVM address.
//
// Version:
//   - 2026-05-24: Added.
//
func NormalizeAddress(address string) (string, error) {
    parsed, err := ParseAddress(address)
    if err != nil {
        return "", fmt.Errorf("failed to normalized evm address: %w", err)
    }

    return parsed.Hex(), nil
}



//
// Parse hex string to EVM address.
//
// Version:
//   - 2026-05-22: Added.
//
func ParseAddress(address string) (common.Address, error) {
    s := strings.TrimSpace(address)
    if s == "" {
        return common.Address{}, fmt.Errorf("failed to parse evm address: missing required parameter: address=%q", "empty")
    }
    if len(s) > 42 {
        return common.Address{}, fmt.Errorf("failed to parse evm address: invalid parameter: address=%q", "too long")
    }
    if !common.IsHexAddress(s) {
        return common.Address{}, fmt.Errorf("failed to parse evm address: invalid parameter: address=%q", s)
    }

    return common.HexToAddress(s), nil
}
