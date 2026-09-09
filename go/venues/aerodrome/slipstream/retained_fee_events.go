package slipstream

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// applyFeeModuleLog applies indexed configuration fields for the selected pool
// and quote origin. Address, block ordering, removals and module replacement
// belong to the producer envelope; removed logs are rejected here as well.
func applyFeeModuleLog(config *dynamicFeeInputs, pool, origin common.Address, log types.Log) error {
	if config == nil {
		return fmt.Errorf("failed to apply retained fee event: configuration=null")
	}
	if log.Removed || len(log.Topics) == 0 || len(log.Data) != 0 {
		return fmt.Errorf("failed to apply retained fee event: event=invalid")
	}
	type eventSpec struct {
		signature string
		scoped    bool
		origin    bool
		value     bool
		max       uint64
	}
	specs := []eventSpec{
		{"CustomFeeSet(address,uint24)", true, false, true, 30000},
		{"ScalingFactorSet(address,uint256)", true, false, true, 1e18},
		{"FeeCapSet(address,uint256)", true, false, true, 50000},
		{"DynamicFeeReset(address)", true, false, false, 0},
		{"DefaultScalingFactorSet(uint256)", false, false, true, 1e18},
		{"DefaultFeeCapSet(uint256)", false, false, true, 50000},
		{"DiscountedRegistered(address,uint24)", true, true, true, 500000},
		{"DiscountedDeregistered(address)", true, true, false, 0},
		{"SecondsAgoSet(uint32)", false, false, true, 131069},
		{"InitialFeeSet(address,uint24)", true, false, true, 50000},
		{"InitialFeeDisabled(address)", true, false, false, 0},
	}
	for _, spec := range specs {
		if log.Topics[0] != crypto.Keccak256Hash([]byte(spec.signature)) {
			continue
		}
		count := 1
		if spec.scoped {
			count++
		}
		if spec.value {
			count++
		}
		if len(log.Topics) != count {
			return fmt.Errorf("failed to apply retained fee event: topics=invalid")
		}
		offset := 1
		if spec.scoped {
			address := log.Topics[offset]
			offset++
			if new(big.Int).SetBytes(address[:12]).Sign() != 0 {
				return fmt.Errorf("failed to apply retained fee event: address=invalid")
			}
			want := pool
			if spec.origin {
				want = origin
			}
			if common.BytesToAddress(address[:]) != want {
				return nil
			}
		}
		var value uint64
		if spec.value {
			n := new(big.Int).SetBytes(log.Topics[offset][:])
			if !n.IsUint64() || n.Uint64() > spec.max {
				return fmt.Errorf("failed to apply retained fee event: value=out_of_range")
			}
			value = n.Uint64()
		}
		next := *config
		switch spec.signature {
		case "CustomFeeSet(address,uint24)":
			next.base = uint32(value)
		case "ScalingFactorSet(address,uint256)":
			next.scaling = value
		case "FeeCapSet(address,uint256)":
			if value == 0 {
				return fmt.Errorf("failed to apply retained fee event: fee_cap=empty")
			}
			next.cap = uint32(value)
		case "DynamicFeeReset(address)":
			next.scaling = 0
			next.cap = 0
			next.initialEnabled = false
			next.initial = 0
		case "DefaultScalingFactorSet(uint256)":
			next.defaultScaling = value
		case "DefaultFeeCapSet(uint256)":
			next.defaultCap = uint32(value)
		case "DiscountedRegistered(address,uint24)":
			next.discount = uint32(value)
		case "DiscountedDeregistered(address)":
			next.discount = 0
		case "SecondsAgoSet(uint32)":
			if value < 2 {
				return fmt.Errorf("failed to apply retained fee event: seconds_ago=out_of_range")
			}
			next.secondsAgo = uint32(value)
		case "InitialFeeSet(address,uint24)":
			next.initialEnabled = true
			next.initial = uint32(value)
		case "InitialFeeDisabled(address)":
			next.initialEnabled = false
			next.initial = 0
		}
		*config = next
		return nil
	}
	return fmt.Errorf("failed to apply retained fee event: event=unsupported")
}
