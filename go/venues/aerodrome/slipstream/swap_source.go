package slipstream

import (
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
)

type SwapSource struct {
	PoolAddress common.Address
	PoolKey     protocol.PoolKey
}

func buildSwapSources(sources []SwapSource) (map[common.Address]protocol.PoolKey, []common.Address, error) {
	if len(sources) == 0 {
		return nil, nil, fmt.Errorf("failed to build slipstream swap sources: sources=empty")
	}
	byAddress := make(map[common.Address]protocol.PoolKey, len(sources))
	addresses := make([]common.Address, 0, len(sources))
	for index, source := range sources {
		if source.PoolAddress == (common.Address{}) {
			return nil, nil, fmt.Errorf("failed to build slipstream swap sources: pool_address=empty source_index=%d", index)
		}
		if err := source.PoolKey.Validate(); err != nil {
			return nil, nil, fmt.Errorf("failed to build slipstream swap sources: %w: source_index=%d", err, index)
		}
		if _, exists := byAddress[source.PoolAddress]; exists {
			return nil, nil, fmt.Errorf("failed to build slipstream swap sources: pool_address=invalid source_index=%d duplicate=true", index)
		}
		byAddress[source.PoolAddress] = source.PoolKey
		addresses = append(addresses, source.PoolAddress)
	}
	sort.Slice(addresses, func(i, j int) bool {
		return bytesCompareAddress(addresses[i], addresses[j]) < 0
	})
	return byAddress, addresses, nil
}

func bytesCompareAddress(a, b common.Address) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
