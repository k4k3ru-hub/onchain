package sui

import (
	"fmt"
	"sort"
	"strings"

	myCore "github.com/k4k3ru-hub/onchain/go/core"
)

type TokenMetadata struct {
	Symbol   string
	CoinType string
	Decimals uint8
}

type PoolMetadata struct {
	Reference      myCore.PoolReference
	Address        Address
	InitialVersion uint64
	TokenA         TokenMetadata
	TokenB         TokenMetadata
}

type Catalog struct {
	entries map[myCore.PoolReference]PoolMetadata
}

// NewCatalog creates an immutable Sui pool metadata catalog.
//
// Parameters:
//   - entries: Sui pool metadata entries.
//
// Returns:
//   - Immutable catalog.
//   - Construction error.
//
// Version:
//   - 2026-08-31: Added.
func NewCatalog(entries []PoolMetadata) (*Catalog, error) {
	catalog := &Catalog{entries: make(map[myCore.PoolReference]PoolMetadata, len(entries))}
	for index, entry := range entries {
		normalized, err := normalizeSuiPoolMetadata(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to create sui pool catalog: %w: entry_index=%d", err, index)
		}
		if _, exists := catalog.entries[normalized.Reference]; exists {
			return nil, fmt.Errorf("failed to create sui pool catalog: pool_reference=invalid duplicate=true entry_index=%d", index)
		}
		catalog.entries[normalized.Reference] = normalized
	}
	return catalog, nil
}

// Resolve resolves Sui pool metadata by its shared reference.
//
// Parameters:
//   - reference: Shared pool reference.
//
// Returns:
//   - Pool metadata.
//   - True when found.
//
// Version:
//   - 2026-08-31: Added.
func (c *Catalog) Resolve(reference myCore.PoolReference) (PoolMetadata, bool) {
	if c == nil {
		return PoolMetadata{}, false
	}
	reference = reference.Normalize()
	if address, err := ParseAddress(reference.PoolID); err == nil {
		reference.PoolID = address.String()
	}
	entry, ok := c.entries[reference]
	return entry, ok
}

// Entries returns all Sui pool metadata in stable reference order.
//
// Returns:
//   - Detached catalog entries.
//
// Version:
//   - 2026-08-31: Added.
func (c *Catalog) Entries() []PoolMetadata {
	if c == nil {
		return nil
	}
	entries := make([]PoolMetadata, 0, len(c.entries))
	for _, entry := range c.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return suiPoolReferenceOrder(entries[i].Reference) < suiPoolReferenceOrder(entries[j].Reference)
	})
	return entries
}

func normalizeSuiPoolMetadata(entry PoolMetadata) (PoolMetadata, error) {
	entry.Reference = entry.Reference.Normalize()
	if err := entry.Reference.Validate(); err != nil {
		return PoolMetadata{}, fmt.Errorf("failed to validate sui pool metadata: %w", err)
	}
	family, err := entry.Reference.Chain.ResolveChainFamily()
	if err != nil || family != myCore.ChainFamilySui {
		return PoolMetadata{}, fmt.Errorf("failed to validate sui pool metadata: chain_family=invalid")
	}
	if entry.Address.IsZero() {
		return PoolMetadata{}, fmt.Errorf("failed to validate sui pool metadata: pool_address=empty")
	}
	if entry.Reference.PoolID != entry.Address.String() {
		parsed, parseErr := ParseAddress(entry.Reference.PoolID)
		if parseErr != nil || parsed != entry.Address {
			return PoolMetadata{}, fmt.Errorf("failed to validate sui pool metadata: pool_id=invalid")
		}
		entry.Reference.PoolID = entry.Address.String()
	}
	if entry.InitialVersion == 0 {
		return PoolMetadata{}, fmt.Errorf("failed to validate sui pool metadata: initial_version=empty")
	}
	entry.TokenA, err = normalizeSuiToken(entry.TokenA, "token_a")
	if err != nil {
		return PoolMetadata{}, err
	}
	entry.TokenB, err = normalizeSuiToken(entry.TokenB, "token_b")
	if err != nil {
		return PoolMetadata{}, err
	}
	if entry.TokenA.CoinType == entry.TokenB.CoinType {
		return PoolMetadata{}, fmt.Errorf("failed to validate sui pool metadata: tokens=invalid duplicate=true")
	}
	return entry, nil
}

func normalizeSuiToken(token TokenMetadata, name string) (TokenMetadata, error) {
	token.Symbol = strings.ToUpper(strings.TrimSpace(token.Symbol))
	if token.Symbol == "" {
		return TokenMetadata{}, fmt.Errorf("failed to validate sui pool metadata: %s_symbol=empty", name)
	}
	if len(token.Symbol) > 32 {
		return TokenMetadata{}, fmt.Errorf("failed to validate sui pool metadata: %s_symbol=too_long actual_length=%d max_length=32", name, len(token.Symbol))
	}
	token.CoinType = strings.TrimSpace(token.CoinType)
	if token.CoinType == "" {
		return TokenMetadata{}, fmt.Errorf("failed to validate sui pool metadata: %s_coin_type=empty", name)
	}
	return token, nil
}

func suiPoolReferenceOrder(reference myCore.PoolReference) string {
	return reference.Chain.String() + "\x00" + reference.Network.String() + "\x00" + string(reference.Protocol) + "\x00" + reference.PoolID
}
