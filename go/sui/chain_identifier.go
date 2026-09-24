package sui

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/k4k3ru-hub/onchain/go/core"
)

type ChainIdentifier string

const (
	ChainIdentifierMainnet ChainIdentifier = "35834a8a"
	ChainIdentifierTestnet ChainIdentifier = "4c78adac"
)

// String returns the Sui chain identifier.
func (i ChainIdentifier) String() string { return string(i) }

// Validate validates a Sui chain identifier.
func (i ChainIdentifier) Validate() error {
	value := string(i)
	if value == "" {
		return fmt.Errorf("failed to validate sui chain identifier: chain_identifier=empty")
	}
	if len(value) != 8 {
		return fmt.Errorf("failed to validate sui chain identifier: chain_identifier=invalid")
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return fmt.Errorf("failed to validate sui chain identifier: chain_identifier=invalid")
		}
	}
	return nil
}

// ChainIdentifier returns the network's canonical eight-character hex identifier.
// GraphQL responses may contain a legacy short identifier or the base58-encoded
// genesis checkpoint digest. Digest responses use their first four bytes.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//
// Returns:
//   - Sui chain identifier.
//   - Retrieval error.
//
// Version:
//   - 2026-09-24: Normalize genesis checkpoint digests while preserving short identifiers.
//   - 2026-08-22: Added.
func (c *RPCClient) ChainIdentifier(ctx context.Context) (ChainIdentifier, error) {
	if c == nil {
		return "", fmt.Errorf("failed to get sui chain identifier: rpc_client=null")
	}
	if c.caller == nil {
		return "", fmt.Errorf("failed to get sui chain identifier: chain_identifier_provider=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result struct {
		ChainIdentifier string `json:"chainIdentifier"`
	}
	if err := c.caller.query(ctx, "query { chainIdentifier }", &result); err != nil {
		return "", fmt.Errorf("failed to get sui chain identifier: %w", err)
	}
	value := result.ChainIdentifier
	if value != "" && len(value) != 8 {
		digest, err := ParseCheckpointDigest(value)
		if err != nil {
			return "", fmt.Errorf("failed to get sui chain identifier: %w", err)
		}
		value = hex.EncodeToString(digest[:4])
	}
	identifier := ChainIdentifier(value)
	if err := identifier.Validate(); err != nil {
		return "", fmt.Errorf("failed to get sui chain identifier: %w", err)
	}
	return identifier, nil
}

// ResolveNetwork resolves a known Sui chain identifier to its network.
//
// Version:
//   - 2026-08-22: Added.
func ResolveNetwork(identifier ChainIdentifier) (core.Network, error) {
	if err := identifier.Validate(); err != nil {
		return "", fmt.Errorf("failed to resolve sui network: %w", err)
	}
	switch identifier {
	case ChainIdentifierMainnet:
		return core.NetworkMainnet, nil
	case ChainIdentifierTestnet:
		return core.NetworkTestnet, nil
	default:
		return "", fmt.Errorf("failed to resolve sui network: chain_identifier=unknown")
	}
}

// ResolveChainIdentifier resolves a supported Sui network to its chain identifier.
//
// Version:
//   - 2026-08-22: Added.
func ResolveChainIdentifier(network core.Network) (ChainIdentifier, error) {
	switch network {
	case core.NetworkMainnet:
		return ChainIdentifierMainnet, nil
	case core.NetworkTestnet:
		return ChainIdentifierTestnet, nil
	default:
		return "", fmt.Errorf("failed to resolve sui chain identifier: network=invalid")
	}
}
