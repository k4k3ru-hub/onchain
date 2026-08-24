package solana

import (
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

const (
	signatureByteLength     = 64
	signatureMaxInputLength = 128
)

type Signature [signatureByteLength]byte

// ParseSignature parses a base58-encoded Solana transaction signature.
//
// Parameters:
//   - value: base58-encoded transaction signature.
//
// Returns:
//   - Parsed Solana transaction signature.
//   - Parse error.
//
// Version:
//   - 2026-08-22: Added.
func ParseSignature(value string) (Signature, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return Signature{}, fmt.Errorf("failed to parse solana signature: signature=empty")
	}
	if len(trimmedValue) > signatureMaxInputLength {
		return Signature{}, fmt.Errorf(
			"failed to parse solana signature: signature=too_long actual_length=%d max_length=%d",
			len(trimmedValue),
			signatureMaxInputLength,
		)
	}

	decoded, err := base58.Decode(trimmedValue)
	if err != nil {
		return Signature{}, fmt.Errorf("failed to parse solana signature: signature=invalid: %w", err)
	}
	if len(decoded) != signatureByteLength {
		return Signature{}, fmt.Errorf(
			"failed to parse solana signature: signature=invalid decoded_length=%d expected_length=%d",
			len(decoded),
			signatureByteLength,
		)
	}

	var signature Signature
	copy(signature[:], decoded)
	return signature, nil
}

// NormalizeSignature normalizes a base58-encoded Solana transaction signature.
//
// Parameters:
//   - value: base58-encoded transaction signature.
//
// Returns:
//   - Canonical base58-encoded transaction signature.
//   - Normalization error.
//
// Version:
//   - 2026-08-22: Added.
func NormalizeSignature(value string) (string, error) {
	signature, err := ParseSignature(value)
	if err != nil {
		return "", fmt.Errorf("failed to normalize solana signature: %w", err)
	}
	return signature.String(), nil
}

// String returns the canonical base58-encoded Solana transaction signature.
//
// Returns:
//   - Canonical base58-encoded transaction signature.
//
// Version:
//   - 2026-08-22: Added.
func (s Signature) String() string {
	return base58.Encode(s[:])
}

// Bytes returns a copy of the Solana transaction signature bytes.
//
// Returns:
//   - Copy of the 64-byte signature.
//
// Version:
//   - 2026-08-22: Added.
func (s Signature) Bytes() []byte {
	result := make([]byte, len(s))
	copy(result, s[:])
	return result
}

// IsZero reports whether the Solana transaction signature contains only zero bytes.
//
// Returns:
//   - True when every signature byte is zero.
//
// Version:
//   - 2026-08-22: Added.
func (s Signature) IsZero() bool {
	return s == Signature{}
}
