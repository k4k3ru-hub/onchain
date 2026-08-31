package solana

import (
	"fmt"
	"sort"
	"strings"

	myCore "github.com/k4k3ru-hub/onchain/go/core"
)

type TokenMetadata struct {
	Symbol   string
	Mint     Address
	Decimals uint8
}

type ProgramMetadata struct {
	ProgramID Address
}

type PoolMetadata struct {
	Reference myCore.PoolReference
	Address   Address
	TokenA    TokenMetadata
	TokenB    TokenMetadata
	VaultA    Address
	VaultB    Address
	Program   ProgramMetadata
}

type Catalog struct {
	entries map[myCore.PoolReference]PoolMetadata
}

// NewCatalog creates an immutable Solana pool metadata catalog.
//
// Parameters:
//   - entries: Solana pool metadata entries.
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
		normalized, err := normalizePoolMetadata(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to create solana pool catalog: %w: entry_index=%d", err, index)
		}
		if _, exists := catalog.entries[normalized.Reference]; exists {
			return nil, fmt.Errorf("failed to create solana pool catalog: pool_reference=invalid duplicate=true entry_index=%d", index)
		}
		catalog.entries[normalized.Reference] = normalized
	}
	return catalog, nil
}

// Resolve resolves Solana pool metadata by its shared reference.
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
	entry, ok := c.entries[reference.Normalize()]
	return entry, ok
}

// Entries returns all Solana pool metadata in stable reference order.
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
		return solanaPoolReferenceOrder(entries[i].Reference) < solanaPoolReferenceOrder(entries[j].Reference)
	})
	return entries
}

func normalizePoolMetadata(entry PoolMetadata) (PoolMetadata, error) {
	entry.Reference = entry.Reference.Normalize()
	if err := entry.Reference.Validate(); err != nil {
		return PoolMetadata{}, fmt.Errorf("failed to validate solana pool metadata: %w", err)
	}
	family, err := entry.Reference.Chain.ResolveChainFamily()
	if err != nil || family != myCore.ChainFamilySolana {
		return PoolMetadata{}, fmt.Errorf("failed to validate solana pool metadata: chain_family=invalid")
	}
	if entry.Address.IsZero() {
		return PoolMetadata{}, fmt.Errorf("failed to validate solana pool metadata: pool_address=empty")
	}
	if entry.Reference.PoolID != entry.Address.String() {
		return PoolMetadata{}, fmt.Errorf("failed to validate solana pool metadata: pool_id=invalid")
	}
	var tokenErr error
	entry.TokenA, tokenErr = normalizeSolanaToken(entry.TokenA, "token_a")
	if tokenErr != nil {
		return PoolMetadata{}, tokenErr
	}
	entry.TokenB, tokenErr = normalizeSolanaToken(entry.TokenB, "token_b")
	if tokenErr != nil {
		return PoolMetadata{}, tokenErr
	}
	if entry.TokenA.Mint == entry.TokenB.Mint {
		return PoolMetadata{}, fmt.Errorf("failed to validate solana pool metadata: tokens=invalid duplicate=true")
	}
	if entry.VaultA.IsZero() {
		return PoolMetadata{}, fmt.Errorf("failed to validate solana pool metadata: vault_a=empty")
	}
	if entry.VaultB.IsZero() {
		return PoolMetadata{}, fmt.Errorf("failed to validate solana pool metadata: vault_b=empty")
	}
	if entry.Program.ProgramID.IsZero() {
		return PoolMetadata{}, fmt.Errorf("failed to validate solana pool metadata: program_id=empty")
	}
	return entry, nil
}

func normalizeSolanaToken(token TokenMetadata, name string) (TokenMetadata, error) {
	token.Symbol = strings.ToUpper(strings.TrimSpace(token.Symbol))
	if token.Symbol == "" {
		return TokenMetadata{}, fmt.Errorf("failed to validate solana pool metadata: %s_symbol=empty", name)
	}
	if len(token.Symbol) > 32 {
		return TokenMetadata{}, fmt.Errorf("failed to validate solana pool metadata: %s_symbol=too_long actual_length=%d max_length=32", name, len(token.Symbol))
	}
	if token.Mint.IsZero() {
		return TokenMetadata{}, fmt.Errorf("failed to validate solana pool metadata: %s_mint=empty", name)
	}
	return token, nil
}

func solanaPoolReferenceOrder(reference myCore.PoolReference) string {
	return reference.Chain.String() + "\x00" + reference.Network.String() + "\x00" + string(reference.Protocol) + "\x00" + reference.PoolID
}
