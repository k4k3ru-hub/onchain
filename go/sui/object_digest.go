package sui

import (
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

type ObjectDigest [digestByteLength]byte

// ParseObjectDigest parses a base58-encoded Sui object digest.
func ParseObjectDigest(value string) (ObjectDigest, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ObjectDigest{}, fmt.Errorf("failed to parse sui object digest: digest=empty")
	}
	decoded, err := base58.Decode(value)
	if err != nil {
		return ObjectDigest{}, fmt.Errorf("failed to parse sui object digest: digest=invalid: %w", err)
	}
	if len(decoded) != digestByteLength {
		return ObjectDigest{}, fmt.Errorf("failed to parse sui object digest: digest=invalid decoded_length=%d expected_length=%d", len(decoded), digestByteLength)
	}
	var digest ObjectDigest
	copy(digest[:], decoded)
	return digest, nil
}

// String returns the base58-encoded Sui object digest.
func (d ObjectDigest) String() string { return base58.Encode(d[:]) }

// Bytes returns a copy of the Sui object digest bytes.
func (d ObjectDigest) Bytes() []byte { return append([]byte(nil), d[:]...) }

// IsZero reports whether the Sui object digest contains only zero bytes.
func (d ObjectDigest) IsZero() bool { return d == ObjectDigest{} }
