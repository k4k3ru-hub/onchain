package slipstream

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

func retainedTestCache() *StateCache {
	p := &poolSnapshot{header: evm.BlockHeader{Number: 100, Hash: common.HexToHash("01"), Timestamp: 100}, observed: time.Unix(100, 0), price: power2(96), liquidity: big.NewInt(1000000000000000000), spacing: 60, fee: 3000, words: map[int32]*big.Int{-1: new(big.Int), 0: new(big.Int)}, ticks: make(map[int32]*big.Int), gross: make(map[int32]*big.Int)}
	return &StateCache{pool: common.HexToAddress("0x30"), factory: common.HexToAddress("0x20"), gate: make(chan struct{}, 1), retained: &retainedPoolState{pool: p, timestamp: 100, fee: retainedFeeState{module: common.HexToAddress("0x10"), config: dynamicFeeInputs{base: 3000, defaultCap: 50000, secondsAgo: 600}, oracle: retainedOracle{slots: []tickObservation{{timestamp: 100, initialized: true}}, next: 1}}}}
}

// TestQuoteRetainedPairUsesNoRPCOrLegacyGate verifies the Trade quote path.
//
// Version:
//   - 2026-09-09: Added.
func TestQuoteRetainedPairUsesNoRPCOrLegacyGate(t *testing.T) {
	c := retainedTestCache() // No RPC dependency and an unavailable legacy gate.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	got, err := c.QuoteRetainedPair(ctx, big.NewInt(1000000), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.BidAmountOut.Sign() <= 0 || got.AskAmountIn.Sign() <= 0 || got.FeePPM != 3000 || got.CapturedAt.Before(start) || got.ObservedAt != time.Unix(100, 0) || got.BlockNumber != 100 {
		t.Fatalf("unexpected pair: %+v", got)
	}
	delete(c.retained.pool.words, -1)
	if _, err := c.QuoteRetainedPair(ctx, big.NewInt(1000000), true); err == nil {
		t.Fatal("missing coverage accepted")
	}
}

// TestRetainedLiquidityUpdatesOracleAndInvalidatesGaps verifies streamed deltas.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedLiquidityUpdatesOracleAndInvalidatesGaps(t *testing.T) {
	c := retainedTestCache()
	word := func(n int64) common.Hash {
		value := big.NewInt(n)
		if n < 0 {
			value.Add(value, power2(256))
		}
		return common.BigToHash(value)
	}
	log := types.Log{Address: c.pool, BlockNumber: 101, BlockHash: common.HexToHash("02"), Topics: []common.Hash{crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)")), {}, word(-60), word(60)}, Data: make([]byte, 128)}
	big.NewInt(100).FillBytes(log.Data[32:64])
	if err := c.applyRetainedLog(log, 102); err != nil {
		t.Fatal(err)
	}
	if c.retained.pool.liquidity.Int64() != 1000000000000000100 || c.retained.pool.ticks[-60].Int64() != 100 || c.retained.pool.ticks[60].Int64() != -100 || c.retained.fee.oracle.slots[0].timestamp != 102 {
		t.Fatal("liquidity/oracle not updated")
	}
	frozen := cloneRetainedPool(c.retained)
	c.retained.pool.ticks[-60].SetInt64(1)
	c.retained.fee.oracle.slots[0].timestamp = 104
	if frozen.pool.ticks[-60].Int64() != 100 || frozen.fee.oracle.slots[0].timestamp != 102 {
		t.Fatal("snapshot aliases live state")
	}
	liquidity := new(big.Int).Set(c.retained.pool.liquidity)
	if err := c.applyRetainedLog(log, 102); err != nil || c.retained.pool.liquidity.Cmp(liquidity) != 0 {
		t.Fatal("duplicate changed liquidity or reset session", err)
	}
	log.Data[63]++
	if err := c.applyRetainedLog(log, 102); err == nil || c.retained != nil {
		t.Fatal("conflicting replay accepted")
	}
}

// TestRetainedFeeWindowExpansionRecoversMissingHistory verifies session recovery rather than a fallback fee.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedFeeWindowExpansionRecoversMissingHistory(t *testing.T) {
	c := retainedTestCache()
	c.retained.fee.oracle = retainedOracle{slots: []tickObservation{{90, 0, true}, {100, 100, true}, {}}, known: []bool{true, true, false}, index: 1, next: 3}
	c.retained.fee.config.secondsAgo = 2
	log := feeEvent("SecondsAgoSet(uint32)", feeValue(20))
	log.Address = c.retained.fee.module
	log.BlockNumber = 101
	log.BlockHash = common.HexToHash("02")
	if err := c.applyRetainedLog(log, 102); err == nil || c.retained != nil {
		t.Fatal("missing expanded history did not reset session")
	}
}
