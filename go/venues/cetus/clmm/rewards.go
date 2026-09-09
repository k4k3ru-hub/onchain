package clmm

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type PoolRewardState struct {
	Pool            sui.Address
	ObjectVersion   uint64
	LastUpdatedTime uint64
	Rewarders       []PoolRewarder
}
type PoolRewarder struct {
	CoinType string
	// EmissionsPerSecondX64 is the raw Q64 rate in token base units per second.
	EmissionsPerSecondX64 *big.Int
	GrowthGlobal          *big.Int
}

// PoolRewards reads the current pool reward manager without calculating APR.
// Prices, TVL, reward funding and yield aggregation remain caller responsibilities.
//
// Version:
//   - 2026-09-09: Added.
func (c *Client) PoolRewards(ctx context.Context, pool sui.Address) (*PoolRewardState, error) {
	if c == nil || c.objects == nil {
		return nil, fmt.Errorf("failed to get cetus pool rewards: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to get cetus pool rewards: context=null")
	}
	if pool.IsZero() {
		return nil, fmt.Errorf("failed to get cetus pool rewards: pool=empty")
	}
	object, err := c.objects.Object(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to get cetus pool rewards: %w", err)
	}
	if object == nil || object.Address != pool {
		return nil, fmt.Errorf("failed to get cetus pool rewards: object=invalid")
	}
	result, err := ParsePoolRewards(object)
	if err != nil {
		return nil, fmt.Errorf("failed to get cetus pool rewards: %w", err)
	}
	return result, nil
}

// ParsePoolRewards decodes reward-manager fields from a full Cetus pool object.
// It accepts flattened GraphQL fields and JSON-RPC fields wrappers.
// Missing managers are errors, while an explicit empty rewarder list is preserved.
//
// Version:
//   - 2026-09-09: Added.
func ParsePoolRewards(object *sui.Object) (*PoolRewardState, error) {
	if object == nil || object.Move == nil {
		return nil, fmt.Errorf("failed to parse cetus pool rewards: object=null")
	}
	if _, _, err := parsePoolType(object.Move.Type); err != nil {
		return nil, fmt.Errorf("failed to parse cetus pool rewards: %w", err)
	}
	root, err := rewardFields(object.Move.JSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cetus pool rewards: %w", err)
	}
	manager, err := rewardFields(root["rewarder_manager"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse cetus pool rewards: %w", err)
	}
	last, err := jsonUint64(manager["last_updated_time"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse cetus pool rewards: %w", err)
	}
	var entries *[]json.RawMessage
	if err := json.Unmarshal(manager["rewarders"], &entries); err != nil {
		return nil, fmt.Errorf("failed to parse cetus pool rewards: %w", err)
	}
	if entries == nil {
		return nil, fmt.Errorf("failed to parse cetus pool rewards: rewarders=null")
	}
	result := &PoolRewardState{Pool: object.Address, ObjectVersion: object.Version, LastUpdatedTime: last, Rewarders: make([]PoolRewarder, 0, len(*entries))}
	for _, entry := range *entries {
		fields, err := rewardFields(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cetus rewarder: %w", err)
		}
		coin, err := rewardFields(fields["reward_coin"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse cetus rewarder: %w", err)
		}
		var name string
		if err := json.Unmarshal(coin["name"], &name); err != nil {
			return nil, fmt.Errorf("failed to parse cetus rewarder: %w", err)
		}
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("failed to parse cetus rewarder: coin_type=empty")
		}
		emission, err := jsonUnsigned(fields["emissions_per_second"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse cetus rewarder: %w", err)
		}
		growth, err := jsonUnsigned(fields["growth_global"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse cetus rewarder: %w", err)
		}
		if emission.BitLen() > 128 || growth.BitLen() > 128 {
			return nil, fmt.Errorf("failed to parse cetus rewarder: value=out_of_range")
		}
		result.Rewarders = append(result.Rewarders, PoolRewarder{CoinType: name, EmissionsPerSecondX64: emission, GrowthGlobal: growth})
	}
	return result, nil
}
func rewardFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("failed to decode cetus reward fields: %w", err)
	}
	if fields == nil {
		return nil, fmt.Errorf("failed to decode cetus reward fields: fields=null")
	}
	if wrapper, ok := fields["fields"]; ok {
		if err := json.Unmarshal(wrapper, &fields); err != nil {
			return nil, fmt.Errorf("failed to decode cetus reward fields: %w", err)
		}
		if fields == nil {
			return nil, fmt.Errorf("failed to decode cetus reward fields: fields=null")
		}
	}
	return fields, nil
}
