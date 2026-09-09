package v3

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// TestRetainedSwapQuotesWithoutRPC verifies stream inputs and detached math.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedSwapQuotesWithoutRPC(t *testing.T) {
	c, rpc := newTestCache(t)
	ctx := context.Background()
	amount := big.NewInt(1000000)
	first, err := c.QuotePair(ctx, amount, true)
	if err != nil {
		t.Fatal(err)
	}
	calls := rpc.calls
	<-c.gate // Simulate a concurrent RPC refresh: local calculation must not wait.
	defer func() { c.gate <- struct{}{} }()
	second, err := c.QuoteRetainedPair(ctx, amount, true)
	if err != nil || first.BidAmountOut.Cmp(second.BidAmountOut) != 0 {
		t.Fatalf("bootstrap mismatch: %v", err)
	}
	log := testSwapLog(t, c.pool)
	log.BlockNumber, log.BlockHash = 101, common.HexToHash("02")
	sqrtAtTick(1).FillBytes(log.Data[64:96])
	c.retained.liquidity.FillBytes(log.Data[96:128])
	big.NewInt(1).FillBytes(log.Data[128:160])
	frozen := clonePoolSnapshot(c.retained)
	if err := c.applyRetainedLog(log); err != nil {
		t.Fatal(err)
	}
	third, err := c.QuoteRetainedPair(ctx, amount, true)
	if err != nil {
		t.Fatal(err)
	}
	if third.BidAmountOut.Cmp(first.BidAmountOut) <= 0 {
		t.Fatal("stream price not used")
	}
	if frozen.price.Cmp(firstPrice()) != 0 {
		t.Fatal("stream mutated frozen calculation")
	}
	if rpc.calls != calls {
		t.Fatal("retained math used RPC")
	}
	if third.BlockNumber != first.BlockNumber || third.BlockHash != first.BlockHash || !third.ObservedAt.Equal(first.ObservedAt) {
		t.Fatal("relabelled baseline")
	}
	c.running = true
	c.retainedBaseAmount = new(big.Int).Set(amount)
	c.retainedRecovery = make(chan struct{}, 1)
	delete(c.retained.words, 0)
	if _, err := c.QuoteRetainedPair(ctx, amount, true); err == nil {
		t.Fatal("missing bitmap accepted")
	}
	if rpc.calls != calls {
		t.Fatal("missing bitmap fetched")
	}
	select {
	case <-c.retainedRecovery:
	default:
		t.Fatal("missing coverage did not request recovery")
	}
	if _, err := c.QuoteRetainedPair(ctx, new(big.Int).Mul(amount, big.NewInt(2)), true); err == nil {
		t.Fatal("missing coverage accepted")
	}
	if len(c.retainedRecovery) != 0 {
		t.Fatal("large query requested reference recovery")
	}
}
func firstPrice() *big.Int { return power2(96) }

func liquidityLog(c *StateCache, mint bool, lower, upper int64, amount int64) types.Log {
	signature := "Burn(address,int24,int24,uint128,uint256,uint256)"
	size, offset := 96, 0
	if mint {
		signature = "Mint(address,address,int24,int24,uint128,uint256,uint256)"
		size, offset = 128, 32
	}
	topic := func(tick int64) common.Hash {
		n := big.NewInt(tick)
		if tick < 0 {
			n.Add(n, power2(256))
		}
		return common.BigToHash(n)
	}
	data := make([]byte, size)
	big.NewInt(amount).FillBytes(data[offset : offset+32])
	return types.Log{Address: c.pool, BlockNumber: 101, BlockHash: common.HexToHash("02"), Index: 1, Topics: []common.Hash{crypto.Keccak256Hash([]byte(signature)), {}, topic(lower), topic(upper)}, Data: data}
}

// TestRetainedLiquidityUpdatesInitializedTicks verifies mint/burn net and bitmap changes.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedLiquidityUpdatesInitializedTicks(t *testing.T) {
	c, _ := newTestCache(t)
	if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	original := new(big.Int).Set(c.retained.liquidity)
	mint := liquidityLog(c, true, -60, 60, 100)
	if err := c.applyRetainedLog(mint); err != nil {
		t.Fatal(err)
	}
	if c.retained.liquidity.Cmp(new(big.Int).Add(original, big.NewInt(100))) != 0 || c.retained.ticks[-60].Int64() != 100 || c.retained.ticks[60].Int64() != -100 {
		t.Fatal("mint delta invalid")
	}
	if c.retained.words[-1].Bit(255) != 1 || c.retained.words[0].Bit(1) != 1 {
		t.Fatal("mint bits not set")
	}
	burn := liquidityLog(c, false, -60, 60, 100)
	burn.Index = 2
	if err := c.applyRetainedLog(burn); err != nil {
		t.Fatal(err)
	}
	if c.retained.liquidity.Cmp(original) != 0 || c.retained.words[-1].Bit(255) != 0 || c.retained.words[0].Bit(1) != 0 {
		t.Fatal("burn delta invalid")
	}
	burn.Removed = true
	if err := c.applyRetainedLog(burn); err == nil || c.retained != nil {
		t.Fatal("removed event retained delta state")
	}
}

// TestRetainedUnknownTickDoesNotInventLiquidity verifies incomplete inputs remain unavailable.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedUnknownTickDoesNotInventLiquidity(t *testing.T) {
	c, _ := newTestCache(t)
	if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
		t.Fatal(err)
	}
	c.retained.words[0].SetBit(c.retained.words[0], 1, 1)
	if err := c.applyRetainedLog(liquidityLog(c, true, -60, 60, 100)); err != nil {
		t.Fatal(err)
	}
	if c.retained.ticks[60] != nil {
		t.Fatal("invented previously unknown liquidity net")
	}
}

// TestRetainedSessionRecoversRemovedLogs verifies initialization and stream termination.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedSessionRecoversRemovedLogs(t *testing.T) {
	c, _ := newTestCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ws := &stateWS{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- c.RunRetained(ctx, ws, big.NewInt(1000000), true) }()
	<-ws.ready
	// Queue removal while bootstrap may still be in progress. The producer must
	// drain it after initialization and discard the freshly captured state too.
	ws.logs <- types.Log{Address: c.pool, BlockNumber: 101, BlockHash: common.HexToHash("02"), Removed: true}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("removed log did not terminate producer")
		}
	case <-time.After(time.Second):
		t.Fatal("producer did not recover")
	}
	if _, err := c.QuoteRetainedPair(ctx, big.NewInt(1000000), true); err == nil {
		t.Fatal("retained removed state")
	}
}

// TestRetainedCoverageEndsSession verifies missing inputs release the producer for reinitialization.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedCoverageEndsSession(t *testing.T) {
	c, rpc := newTestCache(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	amount := big.NewInt(1000000)
	ws := &stateWS{ready: make(chan struct{})}
	err := c.run(ctx, ws, func(ctx context.Context) error {
		if _, err := c.QuotePair(ctx, amount, true); err != nil {
			return err
		}
		c.mu.Lock()
		c.retainedBaseAmount = new(big.Int).Set(amount)
		delete(c.retained.words, 0)
		c.mu.Unlock()
		calls := rpc.calls
		if _, err := c.QuoteRetainedPair(ctx, amount, true); err == nil {
			t.Fatal("missing coverage accepted")
		}
		if rpc.calls != calls {
			t.Fatal("trade quote fetched missing state")
		}
		return nil
	})
	if err == nil || ctx.Err() != nil {
		t.Fatalf("producer did not end for recovery: %v", err)
	}
	if c.running || c.retained != nil {
		t.Fatal("session was not released")
	}
	if _, err := c.QuotePair(context.Background(), amount, true); err != nil {
		t.Fatalf("reinitialization failed: %v", err)
	}
	if _, err := c.QuoteRetainedPair(context.Background(), amount, true); err != nil {
		t.Fatalf("reinitialized local quote failed: %v", err)
	}
}
