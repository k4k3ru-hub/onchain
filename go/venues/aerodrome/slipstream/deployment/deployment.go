package deployment

import (
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
)

type ID string
type Venue string

const (
	IDAerodromeBaseMainnetLegacy  ID = "aerodrome-base-mainnet-legacy"
	IDAerodromeBaseMainnetCurrent ID = "aerodrome-base-mainnet-current"

	VenueAerodrome Venue = "aerodrome"
)

type Deployment struct {
	ID       ID
	ChainID  uint64
	Venue    Venue
	Factory  common.Address
	QuoterV2 common.Address
}

// ByID returns a registered Slipstream deployment.
//
// Parameters:
//   - id: Stable deployment identifier.
//
// Returns:
//   - Registered deployment.
//   - Lookup error.
//
// Version:
//   - 2026-08-30: Added.
func ByID(id ID) (Deployment, error) {
	deployment, ok := registeredDeployments()[id]
	if !ok {
		return Deployment{}, fmt.Errorf("failed to resolve slipstream deployment: deployment_id=invalid")
	}
	return deployment, nil
}

// ListByChainID returns registered Slipstream deployments for a chain.
//
// Parameters:
//   - chainID: EVM chain ID.
//
// Returns:
//   - Deployments ordered by stable identifier.
//
// Version:
//   - 2026-08-30: Added.
func ListByChainID(chainID uint64) []Deployment {
	deployments := make([]Deployment, 0)
	for _, deployment := range registeredDeployments() {
		if deployment.ChainID == chainID {
			deployments = append(deployments, deployment)
		}
	}
	sort.Slice(deployments, func(i, j int) bool { return deployments[i].ID < deployments[j].ID })
	return deployments
}

// LatestByChainID returns the current Slipstream deployment for a chain.
//
// Parameters:
//   - chainID: EVM chain ID.
//
// Returns:
//   - Current deployment.
//   - Lookup error.
//
// Version:
//   - 2026-08-30: Added.
func LatestByChainID(chainID uint64) (Deployment, error) {
	if chainID == baseMainnetChainID {
		return ByID(IDAerodromeBaseMainnetCurrent)
	}
	return Deployment{}, fmt.Errorf("failed to resolve latest slipstream deployment: chain_id=%d", chainID)
}

func registeredDeployments() map[ID]Deployment {
	return map[ID]Deployment{
		IDAerodromeBaseMainnetLegacy:  baseMainnetLegacy(),
		IDAerodromeBaseMainnetCurrent: baseMainnetCurrent(),
	}
}
