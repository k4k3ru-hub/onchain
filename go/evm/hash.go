// hash.go
package evm

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

const (
	hashHexPrefix  = "0x"
	hashByteLength = common.HashLength
	hashHexLength  = len(hashHexPrefix) + hashByteLength*2
)

// ParseHash parses a complete 32-byte EVM hash.
//
// Parameters:
//   - value: 0x-prefixed hexadecimal hash.
//
// Returns:
//   - Parsed EVM hash.
//   - Parse error.
//
// Version:
//   - 2026-08-17: Added.
func ParseHash(value string) (common.Hash, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return common.Hash{}, fmt.Errorf("failed to parse evm hash: hash=empty")
	}

	actualLength := len(trimmedValue)
	if actualLength < hashHexLength {
		return common.Hash{}, fmt.Errorf(
			"failed to parse evm hash: hash=too_short actual_length=%d min_length=%d",
			actualLength,
			hashHexLength,
		)
	}
	if actualLength > hashHexLength {
		return common.Hash{}, fmt.Errorf(
			"failed to parse evm hash: hash=too_long actual_length=%d max_length=%d",
			actualLength,
			hashHexLength,
		)
	}
	if !strings.HasPrefix(trimmedValue, hashHexPrefix) {
		return common.Hash{}, fmt.Errorf("failed to parse evm hash: hash=invalid")
	}

	decoded, err := hex.DecodeString(trimmedValue[len(hashHexPrefix):])
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to parse evm hash: hash=invalid: %w", err)
	}
	if len(decoded) != hashByteLength {
		return common.Hash{}, fmt.Errorf("failed to parse evm hash: hash=invalid")
	}

	return common.BytesToHash(decoded), nil
}

// NormalizeHash normalizes a complete 32-byte EVM hash.
//
// Parameters:
//   - value: 0x-prefixed hexadecimal hash.
//
// Returns:
//   - Normalized EVM hash.
//   - Normalization error.
//
// Version:
//   - 2026-08-17: Added.
func NormalizeHash(value string) (string, error) {
	parsedHash, err := ParseHash(value)
	if err != nil {
		return "", fmt.Errorf("failed to normalize evm hash: %w", err)
	}

	return parsedHash.Hex(), nil
}
