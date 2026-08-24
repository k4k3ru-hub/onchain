package solana

import (
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

const (
	addressByteLength     = 32
	addressMaxInputLength = 64
)

type Address [addressByteLength]byte

// ParseAddress parses a base58-encoded Solana address.
//
// Parameters:
//   - value: base58-encoded address.
//
// Returns:
//   - Parsed Solana address.
//   - Parse error.
//
// Version:
//   - 2026-08-22: Added.
func ParseAddress(value string) (Address, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return Address{}, fmt.Errorf("failed to parse solana address: address=empty")
	}
	if len(trimmedValue) > addressMaxInputLength {
		return Address{}, fmt.Errorf(
			"failed to parse solana address: address=too_long actual_length=%d max_length=%d",
			len(trimmedValue),
			addressMaxInputLength,
		)
	}

	decoded, err := base58.Decode(trimmedValue)
	if err != nil {
		return Address{}, fmt.Errorf("failed to parse solana address: address=invalid: %w", err)
	}
	if len(decoded) != addressByteLength {
		return Address{}, fmt.Errorf(
			"failed to parse solana address: address=invalid decoded_length=%d expected_length=%d",
			len(decoded),
			addressByteLength,
		)
	}

	var address Address
	copy(address[:], decoded)
	return address, nil
}

// NormalizeAddress normalizes a base58-encoded Solana address.
//
// Parameters:
//   - value: base58-encoded address.
//
// Returns:
//   - Canonical base58-encoded address.
//   - Normalization error.
//
// Version:
//   - 2026-08-22: Added.
func NormalizeAddress(value string) (string, error) {
	address, err := ParseAddress(value)
	if err != nil {
		return "", fmt.Errorf("failed to normalize solana address: %w", err)
	}
	return address.String(), nil
}

// String returns the canonical base58-encoded Solana address.
//
// Returns:
//   - Canonical base58-encoded address.
//
// Version:
//   - 2026-08-22: Added.
func (a Address) String() string {
	return base58.Encode(a[:])
}

// Bytes returns a copy of the Solana address bytes.
//
// Returns:
//   - Copy of the 32-byte address.
//
// Version:
//   - 2026-08-22: Added.
func (a Address) Bytes() []byte {
	result := make([]byte, len(a))
	copy(result, a[:])
	return result
}

// IsZero reports whether the Solana address contains only zero bytes.
//
// Returns:
//   - True when every address byte is zero.
//
// Version:
//   - 2026-08-22: Added.
func (a Address) IsZero() bool {
	return a == Address{}
}
