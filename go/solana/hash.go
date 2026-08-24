package solana

import (
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

const hashByteLength = 32

type Hash [hashByteLength]byte

// ParseHash parses a base58-encoded Solana hash.
//
// Parameters:
//   - value: base58-encoded hash.
//
// Returns:
//   - Parsed hash.
//   - Parse error.
//
// Version:
//   - 2026-08-22: Added.
func ParseHash(value string) (Hash, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Hash{}, fmt.Errorf("failed to parse solana hash: hash=empty")
	}
	decoded, err := base58.Decode(value)
	if err != nil {
		return Hash{}, fmt.Errorf("failed to parse solana hash: hash=invalid: %w", err)
	}
	if len(decoded) != hashByteLength {
		return Hash{}, fmt.Errorf("failed to parse solana hash: hash=invalid decoded_length=%d expected_length=%d", len(decoded), hashByteLength)
	}
	var hash Hash
	copy(hash[:], decoded)
	return hash, nil
}

// String returns the canonical base58-encoded Solana hash.
func (h Hash) String() string { return base58.Encode(h[:]) }

// Bytes returns a copy of the Solana hash bytes.
func (h Hash) Bytes() []byte { return append([]byte(nil), h[:]...) }

// IsZero reports whether the Solana hash contains only zero bytes.
func (h Hash) IsZero() bool { return h == Hash{} }
