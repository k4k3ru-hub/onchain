package sui

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const addressByteLength = 32

type Address [addressByteLength]byte

// ParseAddress parses a hexadecimal Sui address.
//
// Parameters:
//   - value: hexadecimal address with or without the 0x prefix.
//
// Returns:
//   - Parsed Sui address.
//   - Parse error.
//
// Version:
//   - 2026-08-22: Added.
func ParseAddress(value string) (Address, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Address{}, fmt.Errorf("failed to parse sui address: address=empty")
	}
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	if value == "" || len(value) > addressByteLength*2 {
		return Address{}, fmt.Errorf("failed to parse sui address: address=invalid")
	}
	if len(value)%2 != 0 {
		value = "0" + value
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return Address{}, fmt.Errorf("failed to parse sui address: address=invalid: %w", err)
	}
	var address Address
	copy(address[addressByteLength-len(decoded):], decoded)
	return address, nil
}

// NormalizeAddress normalizes a hexadecimal Sui address.
func NormalizeAddress(value string) (string, error) {
	address, err := ParseAddress(value)
	if err != nil {
		return "", fmt.Errorf("failed to normalize sui address: %w", err)
	}
	return address.String(), nil
}

// String returns the canonical 32-byte hexadecimal Sui address.
func (a Address) String() string { return "0x" + hex.EncodeToString(a[:]) }

// MarshalText returns the canonical hexadecimal address representation.
//
// Version:
//   - 2026-08-23: Added.
func (a Address) MarshalText() ([]byte, error) { return []byte(a.String()), nil }

// Bytes returns a copy of the Sui address bytes.
func (a Address) Bytes() []byte { return append([]byte(nil), a[:]...) }

// IsZero reports whether the Sui address contains only zero bytes.
func (a Address) IsZero() bool { return a == Address{} }
