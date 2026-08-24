package sui

import (
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

type CheckpointDigest [digestByteLength]byte

// ParseCheckpointDigest parses a base58-encoded Sui checkpoint digest.
func ParseCheckpointDigest(value string) (CheckpointDigest, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return CheckpointDigest{}, fmt.Errorf("failed to parse sui checkpoint digest: digest=empty")
	}
	decoded, err := base58.Decode(value)
	if err != nil {
		return CheckpointDigest{}, fmt.Errorf("failed to parse sui checkpoint digest: digest=invalid: %w", err)
	}
	if len(decoded) != digestByteLength {
		return CheckpointDigest{}, fmt.Errorf("failed to parse sui checkpoint digest: digest=invalid decoded_length=%d expected_length=%d", len(decoded), digestByteLength)
	}
	var digest CheckpointDigest
	copy(digest[:], decoded)
	return digest, nil
}

// String returns the base58-encoded Sui checkpoint digest.
func (d CheckpointDigest) String() string { return base58.Encode(d[:]) }

// Bytes returns a copy of the Sui checkpoint digest bytes.
func (d CheckpointDigest) Bytes() []byte { return append([]byte(nil), d[:]...) }

// IsZero reports whether the Sui checkpoint digest contains only zero bytes.
func (d CheckpointDigest) IsZero() bool { return d == CheckpointDigest{} }
