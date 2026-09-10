package v4

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
//   - 2026-09-10: Preserve input receipt time.
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
	log := types.Log{Address: c.pool, BlockNumber: 101, BlockHash: common.HexToHash("02"), Topics: []common.Hash{crypto.Keccak256Hash([]byte("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)")), c.poolID, {}}, Data: make([]byte, 192)}
	sqrtAtTick(1).FillBytes(log.Data[64:96])
	c.retained.liquidity.FillBytes(log.Data[96:128])
	big.NewInt(1).FillBytes(log.Data[128:160])
	big.NewInt(3000).FillBytes(log.Data[160:192])
	frozen := clonePoolSnapshot(c.retained)
	received := first.ReceivedAt.Add(time.Second)
	if err := c.applyRetainedLog(log, received); err != nil {
		t.Fatal(err)
	}
	third, err := c.QuoteRetainedPair(ctx, amount, true)
	if err != nil {
		t.Fatal(err)
	}
	if first.ReceivedAt.IsZero() || !second.ReceivedAt.Equal(first.ReceivedAt) || !third.ReceivedAt.Equal(received) {
		t.Fatal("receipt changed during calculation or replay")
	}
	old := log
	old.BlockNumber, old.BlockHash = first.BlockNumber, first.BlockHash
	if err := c.applyRetainedLog(old, received.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !c.retained.received.Equal(received) {
		t.Fatal("baseline replay refreshed receipt")
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
	delete(c.retained.words, 0)
	if _, err := c.QuoteRetainedPair(ctx, amount, true); err == nil {
		t.Fatal("missing bitmap accepted")
	}
	if rpc.calls != calls {
		t.Fatal("missing bitmap fetched")
	}
}
func firstPrice() *big.Int { return power2(96) }

func liquidityLog(c *StateCache, mint bool, lower, upper int64, amount int64) types.Log {
	word := func(value int64) []byte {
		n := big.NewInt(value)
		if value < 0 {
			n.Add(n, power2(256))
		}
		return n.FillBytes(make([]byte, 32))
	}
	if !mint {
		amount = -amount
	}
	data := append(word(lower), word(upper)...)
	data = append(data, word(amount)...)
	data = append(data, make([]byte, 32)...)
	return types.Log{Address: c.pool, BlockNumber: 101, BlockHash: common.HexToHash("02"), Index: 1, Topics: []common.Hash{crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)")), c.poolID, {}}, Data: data}
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
	ws.logs <- types.Log{Address: c.pool, BlockNumber: 101, BlockHash: common.HexToHash("02"), Removed: true, Topics: []common.Hash{{}, c.poolID}}
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

// TestRetainedProtocolFees verifies streamed directional fees affect local math.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedProtocolFees(t *testing.T) {
	c, rpc := newTestCache(t)
	amount := big.NewInt(1000000)
	first, err := c.QuotePair(context.Background(), amount, true)
	if err != nil {
		t.Fatal(err)
	}
	calls := rpc.calls
	log := types.Log{Address: c.pool, BlockNumber: 101, BlockHash: common.HexToHash("02"), Topics: []common.Hash{crypto.Keccak256Hash([]byte("ProtocolFeeUpdated(bytes32,uint24)")), c.poolID}, Data: big.NewInt(500 | 1000<<12).FillBytes(make([]byte, 32))}
	if err := c.applyRetainedLog(log); err != nil {
		t.Fatal(err)
	}
	if c.retained.fees != [2]uint32{3499, 3997} {
		t.Fatalf("fees: %v", c.retained.fees)
	}
	second, err := c.QuoteRetainedPair(context.Background(), amount, true)
	if err != nil {
		t.Fatal(err)
	}
	if second.BidAmountOut.Cmp(first.BidAmountOut) >= 0 || second.AskAmountIn.Cmp(first.AskAmountIn) <= 0 || rpc.calls != calls {
		t.Fatal("stream fees not applied locally")
	}
	log.Index++
	log.Data = big.NewInt(1001).FillBytes(make([]byte, 32))
	if err := c.applyRetainedLog(log); err == nil || c.retained != nil {
		t.Fatal("invalid fee retained")
	}
}
