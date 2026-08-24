package sui

import (
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

const digestByteLength = 32

type TransactionDigest [digestByteLength]byte

// ParseTransactionDigest parses a base58-encoded Sui transaction digest.
func ParseTransactionDigest(value string) (TransactionDigest, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return TransactionDigest{}, fmt.Errorf("failed to parse sui transaction digest: digest=empty")
	}
	decoded, err := base58.Decode(value)
	if err != nil {
		return TransactionDigest{}, fmt.Errorf("failed to parse sui transaction digest: digest=invalid: %w", err)
	}
	if len(decoded) != digestByteLength {
		return TransactionDigest{}, fmt.Errorf("failed to parse sui transaction digest: digest=invalid decoded_length=%d expected_length=%d", len(decoded), digestByteLength)
	}
	var digest TransactionDigest
	copy(digest[:], decoded)
	return digest, nil
}

// String returns the base58-encoded Sui transaction digest.
func (d TransactionDigest) String() string { return base58.Encode(d[:]) }

// MarshalText returns the base58 transaction digest representation.
//
// Version:
//   - 2026-08-23: Added.
func (d TransactionDigest) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// Bytes returns a copy of the Sui transaction digest bytes.
func (d TransactionDigest) Bytes() []byte { return append([]byte(nil), d[:]...) }

// IsZero reports whether the Sui transaction digest contains only zero bytes.
func (d TransactionDigest) IsZero() bool { return d == TransactionDigest{} }
