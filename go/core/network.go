// network.go
package core

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Network string

const (
	NetworkMainnet Network = "mainnet"
	NetworkTestnet Network = "testnet"
	NetworkDevnet  Network = "devnet"
	NetworkSepolia Network = "sepolia"
	NetworkHolesky Network = "holesky"
	NetworkAmoy    Network = "amoy"
)

// IsValid reports whether Validate accepts the network identifier.
// It does not establish chain, adapter or deployment support.
//
// Version:
//   - 2026-09-25: Delegate to format validation instead of a known-network list.
//   - 2026-08-22: Added Amoy.
//   - 2026-05-17: Added.
func (n Network) IsValid() bool {
	return n.Validate() == nil
}

// Validate validates a nonempty UTF-8 network identifier of at most 16 bytes.
// Whitespace and control characters are rejected; case is preserved.
// Chain combinations and deployment support must be checked by the consumer.
//
// Version:
//   - 2026-09-25: Validate identifier format without restricting names to constants.
//   - 2026-08-22: Added Amoy.
//   - 2026-05-17: Added.
func (n Network) Validate() error {
	if n == "" {
		return fmt.Errorf("failed to validate network: network=empty")
	}
	if len(n) > 16 {
		return fmt.Errorf("failed to validate network: network=too_long actual_length=%d max_length=16", len(n))
	}
	if !utf8.ValidString(string(n)) || strings.IndexFunc(string(n), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return fmt.Errorf("failed to validate network: network=invalid")
	}
	return nil
}

// Convert network to string.
//
// Version:
//   - 2026-05-17: Added.
func (n Network) String() string {
	return string(n)
}
