package slipstream

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"testing"
)

func feeEvent(signature string, values ...common.Hash) types.Log {
	return types.Log{Topics: append([]common.Hash{crypto.Keccak256Hash([]byte(signature))}, values...)}
}
func feeValue(v int64) common.Hash { return common.BigToHash(big.NewInt(v)) }

// TestFeeModuleEvents verifies fee changes, origin isolation and rejection without partial mutation.
//
// Version:
//   - 2026-09-09: Added.
func TestFeeModuleEvents(t *testing.T) {
	pool := common.HexToAddress("0x123")
	origin := common.HexToAddress("0x456")
	poolTopic := common.BytesToHash(pool.Bytes())
	originTopic := common.BytesToHash(origin.Bytes())
	config := dynamicFeeInputs{base: 3000, secondsAgo: 600}
	for _, log := range []types.Log{
		feeEvent("DefaultScalingFactorSet(uint256)", feeValue(1000000)),
		feeEvent("DefaultFeeCapSet(uint256)", feeValue(50000)),
		feeEvent("CustomFeeSet(address,uint24)", poolTopic, feeValue(500)),
		feeEvent("FeeCapSet(address,uint256)", poolTopic, feeValue(20000)),
		feeEvent("ScalingFactorSet(address,uint256)", poolTopic, feeValue(2000000)),
		feeEvent("SecondsAgoSet(uint32)", feeValue(2)),
		feeEvent("DiscountedRegistered(address,uint24)", originTopic, feeValue(500000)),
	} {
		if err := applyFeeModuleLog(&config, pool, origin, log); err != nil {
			t.Fatal(err)
		}
	}
	oracle := retainedOracle{slots: []tickObservation{{100, 0, true}, {102, 20, true}}, index: 1, next: 2}
	got, err := oracle.fee(config, 102, 100)
	if err != nil || got != 340 {
		t.Fatalf("fee=%d err=%v", got, err)
	}
	before := config
	if err := applyFeeModuleLog(&config, pool, origin, feeEvent("CustomFeeSet(address,uint24)", originTopic, feeValue(420))); err != nil || config != before {
		t.Fatal("other pool changed state", err)
	}
	for _, log := range []types.Log{feeEvent("FeeCapSet(address,uint256)", poolTopic, feeValue(0)), feeEvent("SecondsAgoSet(uint32)", feeValue(1)), feeEvent("CustomFeeSet(address,uint24)", poolTopic, feeValue(30001)), feeEvent("Unknown()"), {Removed: true, Topics: []common.Hash{}}} {
		if err := applyFeeModuleLog(&config, pool, origin, log); err == nil || config != before {
			t.Fatal("invalid event accepted or mutated config")
		}
	}
	for _, log := range []types.Log{feeEvent("InitialFeeSet(address,uint24)", poolTopic, feeValue(420)), feeEvent("InitialFeeDisabled(address)", poolTopic), feeEvent("DiscountedDeregistered(address)", originTopic), feeEvent("DynamicFeeReset(address)", poolTopic)} {
		if err := applyFeeModuleLog(&config, pool, origin, log); err != nil {
			t.Fatal(err)
		}
	}
	if config.initialEnabled || config.initial != 0 || config.discount != 0 || config.scaling != 0 || config.cap != 0 || config.base != 500 {
		t.Fatalf("reset: %+v", config)
	}
}
