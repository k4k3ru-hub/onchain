package evm

import (
	"fmt"
	"strconv"

	"github.com/k4k3ru-hub/onchain/go/core"
)

type ChainID uint64

const (
	ChainIDEthereumMainnet ChainID = 1
	ChainIDBNBMainnet      ChainID = 56
	ChainIDBaseMainnet     ChainID = 8453
)

type ChainNetwork struct {
	Chain   core.Chain
	Network core.Network
}

type chainDefinition struct {
	chainID ChainID
	chain   core.Chain
	network core.Network
}

var chainDefinitions = [...]chainDefinition{
	{
		chainID: ChainIDEthereumMainnet,
		chain:   core.ChainEthereum,
		network: core.NetworkMainnet,
	},
	{
		chainID: ChainIDBNBMainnet,
		chain:   core.ChainBNB,
		network: core.NetworkMainnet,
	},
	{
		chainID: ChainIDBaseMainnet,
		chain:   core.ChainBase,
		network: core.NetworkMainnet,
	},
}

// Uint64 returns the numeric EVM chain ID.
//
// Returns:
//   - Numeric EVM chain ID.
//
// Version:
//   - 2026-08-19: Added.
func (id ChainID) Uint64() uint64 {
	return uint64(id)
}

// String returns the decimal EVM chain ID.
//
// Returns:
//   - Decimal EVM chain ID.
//
// Version:
//   - 2026-08-19: Added.
func (id ChainID) String() string {
	return strconv.FormatUint(id.Uint64(), 10)
}

// Validate validates a supported EVM chain ID.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-19: Added.
func (id ChainID) Validate() error {
	if id == 0 {
		return fmt.Errorf("failed to validate evm chain id: chain_id=empty")
	}
	if _, ok := findChainDefinitionByID(id); !ok {
		return fmt.Errorf("failed to validate evm chain id: chain_id=invalid")
	}
	return nil
}

// ResolveChainID resolves an EVM chain ID.
//
// Parameters:
//   - chain: blockchain.
//   - network: blockchain network.
//
// Returns:
//   - EVM chain ID.
//   - Resolution error.
//
// Version:
//   - 2026-08-19: Added.
func ResolveChainID(chain core.Chain, network core.Network) (ChainID, error) {
	if err := chain.Validate(); err != nil {
		return 0, fmt.Errorf("failed to resolve evm chain id: %w", err)
	}
	if err := network.Validate(); err != nil {
		return 0, fmt.Errorf("failed to resolve evm chain id: %w", err)
	}

	family, err := chain.ResolveChainFamily()
	if err != nil {
		return 0, fmt.Errorf("failed to resolve evm chain id: %w", err)
	}
	if family != core.ChainFamilyEVM {
		return 0, fmt.Errorf("failed to resolve evm chain id: chain_family=invalid: chain=%q", chain)
	}

	definition, ok := findChainDefinition(chain, network)
	if !ok {
		return 0, fmt.Errorf(
			"failed to resolve evm chain id: chain network combination is unsupported: chain=%q network=%q",
			chain,
			network,
		)
	}
	return definition.chainID, nil
}

// ResolveChainNetwork resolves a blockchain and network from an EVM chain ID.
//
// Parameters:
//   - chainID: EVM chain ID.
//
// Returns:
//   - Blockchain and network.
//   - Resolution error.
//
// Version:
//   - 2026-08-19: Added.
func ResolveChainNetwork(chainID ChainID) (ChainNetwork, error) {
	if chainID == 0 {
		return ChainNetwork{}, fmt.Errorf("failed to resolve chain network from evm chain id: chain_id=empty")
	}

	definition, ok := findChainDefinitionByID(chainID)
	if !ok {
		return ChainNetwork{}, fmt.Errorf("failed to resolve chain network from evm chain id: chain_id=invalid")
	}
	return ChainNetwork{
		Chain:   definition.chain,
		Network: definition.network,
	}, nil
}

func findChainDefinition(chain core.Chain, network core.Network) (chainDefinition, bool) {
	for _, definition := range chainDefinitions {
		if definition.chain == chain && definition.network == network {
			return definition, true
		}
	}
	return chainDefinition{}, false
}

func findChainDefinitionByID(chainID ChainID) (chainDefinition, bool) {
	for _, definition := range chainDefinitions {
		if definition.chainID == chainID {
			return definition, true
		}
	}
	return chainDefinition{}, false
}
