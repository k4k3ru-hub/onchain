package slipstream

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"testing"
	"time"
)

// TestLiveReplacementPreservesRetainedInputs accepts hash and position changes
// without RPC or historical Oracle writes.
//
// Version:
//   - 2026-09-12: Added.
func TestLiveReplacementPreservesRetainedInputs(t *testing.T) {
	c := retainedTestCache()
	c.retained.block, c.retained.hash, c.retained.index, c.retained.hasLog = 101, common.HexToHash("02"), 59, true
	log := types.Log{Address: c.pool, BlockNumber: 101, BlockHash: common.HexToHash("03"), Index: 44, Topics: []common.Hash{swapEventSignatureHash(), {}, {}}, Data: make([]byte, 160)}
	big.NewInt(1).FillBytes(log.Data[:32])
	amount := new(big.Int).Sub(power2(256), big.NewInt(1))
	amount.FillBytes(log.Data[32:64])
	sqrtAtTick(1).FillBytes(log.Data[64:96])
	c.retained.pool.liquidity.FillBytes(log.Data[96:128])
	big.NewInt(1).FillBytes(log.Data[128:160])
	if err := c.applyLiveRetainedLog(log, 99, time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	if c.retained == nil || c.retained.pool.tick != 1 || c.retained.index != 44 || c.retained.fee.oracle.slots[0].timestamp != 100 {
		t.Fatal("replacement did not retain inputs")
	}
	if _, err := c.QuoteRetainedPair(context.Background(), big.NewInt(1), true); err != nil {
		t.Fatal(err)
	}
	log.Removed = true
	if err := c.applyLiveRetainedLog(log, 99, time.Unix(201, 0)); err != nil || c.retained == nil {
		t.Fatal("cancellation discarded state", err)
	}
}
