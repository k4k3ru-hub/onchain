package v4

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"testing"
	"time"
)

// TestLiveSwapReceiveOrder preserves local quotes across ledger replacements.
//
// Version:
//   - 2026-09-12: Added.
func TestLiveSwapReceiveOrder(t *testing.T) {
	c, rpc := newTestCache(t)
	amount := big.NewInt(1000000)
	if _, err := c.QuotePair(context.Background(), amount, true); err != nil {
		t.Fatal(err)
	}
	calls := rpc.calls
	log := types.Log{Address: c.pool, Topics: []common.Hash{crypto.Keccak256Hash([]byte("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)")), c.poolID, {}}, Data: make([]byte, 192)}
	big.NewInt(3000).FillBytes(log.Data[160:192])
	c.retained.liquidity.FillBytes(log.Data[96:128])
	var order retainedLogOrder
	for i, index := range []uint{59, 44, 2} {
		log.BlockNumber = uint64(101 - i/2)
		log.BlockHash = common.BigToHash(big.NewInt(int64(i + 10)))
		log.Index = index
		sqrtAtTick(int32(i + 1)).FillBytes(log.Data[64:96])
		big.NewInt(int64(i + 1)).FillBytes(log.Data[128:160])
		if duplicate, err := order.accept(log); duplicate || err != nil {
			t.Fatal(duplicate, err)
		}
		if err := c.applyLiveRetainedLog(log, time.Now()); err != nil {
			t.Fatal(err)
		}
		if c.retained == nil || c.retained.tick != int32(i+1) {
			t.Fatal("terminal swap not applied")
		}
		if _, err := c.QuoteRetainedPair(context.Background(), amount, true); err != nil {
			t.Fatal(err)
		}
		if duplicate, err := order.accept(log); !duplicate || err != nil {
			t.Fatal("duplicate not suppressed", err)
		}
	}
	before := c.retained
	log.Removed = true
	if duplicate, err := order.accept(log); duplicate || err != nil {
		t.Fatal("cancellation suppressed", err)
	}
	if err := c.applyLiveRetainedLog(log, time.Now()); err != nil || c.retained != before {
		t.Fatal("cancellation invalidated state", err)
	}
	if rpc.calls != calls {
		t.Fatal("live swap or quote used RPC")
	}
}

// TestRetainedWindowRecentersBeforeQuoteFailure detects the outer word even when
// opportunistic quote reads have populated additional neighboring words.
//
// Version:
//   - 2026-09-12: Added.
func TestRetainedWindowRecentersBeforeQuoteFailure(t *testing.T) {
	c, _ := newTestCache(t)
	s, err := c.captureRetainedWindow(context.Background(), big.NewInt(1000000), true, 0, common.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	c.retained = s
	c.retained.tick = (s.windowCenter + 1) * 256 * s.spacing
	c.retained.price = sqrtAtTick(c.retained.tick)
	c.retained.words[s.windowCenter+2] = new(big.Int)
	if !c.retainedNeedsCapture(big.NewInt(1), true) {
		t.Fatal("outer word did not trigger prefetch")
	}
}
