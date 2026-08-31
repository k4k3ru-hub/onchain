package evm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	myCore "github.com/k4k3ru-hub/onchain/go/core"
)

type TokenMetadata struct {
	Symbol   string
	Address  string
	Decimals uint8
}

type DeploymentMetadata struct {
	Router      string
	Quoter      string
	PoolManager string
}

type PoolMetadata struct {
	Reference   myCore.PoolReference
	Address     string
	Token0      TokenMetadata
	Token1      TokenMetadata
	Fee         uint32
	TickSpacing int32
	Hooks       string
	Deployment  DeploymentMetadata
}

type Catalog struct {
	entries map[myCore.PoolReference]PoolMetadata
}

// NewCatalog creates an immutable EVM pool metadata catalog.
//
// Parameters:
//   - entries: EVM pool metadata entries.
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
			return nil, fmt.Errorf("failed to create evm pool catalog: %w: entry_index=%d", err, index)
		}
		if _, exists := catalog.entries[normalized.Reference]; exists {
			return nil, fmt.Errorf("failed to create evm pool catalog: pool_reference=invalid duplicate=true entry_index=%d", index)
		}
		catalog.entries[normalized.Reference] = normalized
	}
	return catalog, nil
}

// Resolve resolves EVM pool metadata by its shared reference.
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
	if common.IsHexAddress(reference.PoolID) {
		reference.PoolID = common.HexToAddress(reference.PoolID).Hex()
	}
	entry, ok := c.entries[reference]
	return entry, ok
}

// Entries returns all EVM pool metadata in stable reference order.
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
		return poolReferenceOrder(entries[i].Reference) < poolReferenceOrder(entries[j].Reference)
	})
	return entries
}

func normalizePoolMetadata(entry PoolMetadata) (PoolMetadata, error) {
	entry.Reference = entry.Reference.Normalize()
	if err := entry.Reference.Validate(); err != nil {
		return PoolMetadata{}, fmt.Errorf("failed to validate evm pool metadata: %w", err)
	}
	family, err := entry.Reference.Chain.ResolveChainFamily()
	if err != nil || family != myCore.ChainFamilyEVM {
		return PoolMetadata{}, fmt.Errorf("failed to validate evm pool metadata: chain_family=invalid")
	}
	entry.Address, err = normalizeRequiredEVMAddress(entry.Address, "pool_address")
	if err != nil {
		return PoolMetadata{}, err
	}
	if !common.IsHexAddress(entry.Reference.PoolID) || common.HexToAddress(entry.Reference.PoolID) != common.HexToAddress(entry.Address) {
		return PoolMetadata{}, fmt.Errorf("failed to validate evm pool metadata: pool_id=invalid")
	}
	entry.Reference.PoolID = entry.Address
	entry.Token0, err = normalizeEVMToken(entry.Token0, "token0")
	if err != nil {
		return PoolMetadata{}, err
	}
	entry.Token1, err = normalizeEVMToken(entry.Token1, "token1")
	if err != nil {
		return PoolMetadata{}, err
	}
	if entry.Token0.Address == entry.Token1.Address {
		return PoolMetadata{}, fmt.Errorf("failed to validate evm pool metadata: tokens=invalid duplicate=true")
	}
	for name, value := range map[string]*string{"router": &entry.Deployment.Router, "quoter": &entry.Deployment.Quoter, "pool_manager": &entry.Deployment.PoolManager, "hooks": &entry.Hooks} {
		if strings.TrimSpace(*value) == "" {
			*value = ""
			continue
		}
		*value, err = normalizeRequiredEVMAddress(*value, name)
		if err != nil {
			return PoolMetadata{}, err
		}
	}
	return entry, nil
}

func normalizeEVMToken(token TokenMetadata, name string) (TokenMetadata, error) {
	token.Symbol = strings.ToUpper(strings.TrimSpace(token.Symbol))
	if token.Symbol == "" {
		return TokenMetadata{}, fmt.Errorf("failed to validate evm pool metadata: %s_symbol=empty", name)
	}
	if len(token.Symbol) > 32 {
		return TokenMetadata{}, fmt.Errorf("failed to validate evm pool metadata: %s_symbol=too_long actual_length=%d max_length=32", name, len(token.Symbol))
	}
	address, err := normalizeRequiredEVMAddress(token.Address, name+"_address")
	if err != nil {
		return TokenMetadata{}, err
	}
	token.Address = address
	return token, nil
}

func normalizeRequiredEVMAddress(value, name string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("failed to validate evm pool metadata: %s=empty", name)
	}
	if !common.IsHexAddress(value) {
		return "", fmt.Errorf("failed to validate evm pool metadata: %s=invalid", name)
	}
	return common.HexToAddress(value).Hex(), nil
}

func poolReferenceOrder(reference myCore.PoolReference) string {
	return reference.Chain.String() + "\x00" + reference.Network.String() + "\x00" + string(reference.Protocol) + "\x00" + reference.PoolID
}
