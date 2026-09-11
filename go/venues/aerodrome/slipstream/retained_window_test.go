package slipstream

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

// TestInstallRetainedWindowReplaysAndRollsBack verifies concurrent liquidity is preserved.
//
// Version:
//   - 2026-09-11: Added.
func TestInstallRetainedWindowReplaysAndRollsBack(t *testing.T) {
	c := retainedTestCache()
	candidate := cloneRetainedPool(c.retained)
	candidate.pool.words[1] = new(big.Int)
	lower := new(big.Int).Sub(power2(256), big.NewInt(60))
	log := types.Log{Address: c.pool, BlockNumber: 101, BlockHash: common.HexToHash("02"), Index: 5, Topics: []common.Hash{crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)")), {}, common.BigToHash(lower), common.BigToHash(big.NewInt(60))}, Data: make([]byte, 128)}
	big.NewInt(100).FillBytes(log.Data[32:64])
	receipt := time.Unix(103, 0)
	if err := c.applyRetainedLog(log, 102, receipt); err != nil {
		t.Fatal(err)
	}
	expected := new(big.Int).Set(c.retained.pool.liquidity)
	if err := c.installRetainedWindow(candidate, []retainedWindowLog{{log, 102, receipt}}); err != nil {
		t.Fatal(err)
	}
	if c.retained.pool.liquidity.Cmp(expected) != 0 || c.retained.received != receipt || c.retained.pool.words[1] == nil {
		t.Fatal("lost live state or candidate coverage")
	}
	before := c.retained
	bad := cloneRetainedPool(before)
	log.Removed = true
	if err := c.installRetainedWindow(bad, []retainedWindowLog{{log, 102, receipt}}); err == nil || c.retained != before {
		t.Fatal("failed replay replaced live state")
	}
	stale := cloneRetainedPool(before)
	stale.hasLog = false
	if err := c.installRetainedWindow(stale, nil); err == nil || c.retained != before {
		t.Fatal("regressed candidate replaced live state")
	}
}

// TestRetainedWindowRecentersWithoutRPC verifies ±1 word movement and complete tick checks.
//
// Version:
//   - 2026-09-11: Added.
func TestRetainedWindowRecentersWithoutRPC(t *testing.T) {
	c := retainedTestCache()
	c.retained.pool.words[1] = new(big.Int)
	if c.retainedNeedsCapture(big.NewInt(1000000), true) {
		t.Fatal("complete window requested capture")
	}
	c.retained.pool.tick = -1
	c.retained.pool.price = sqrtAtTick(-1)
	if !c.retainedNeedsCapture(big.NewInt(1000000), true) {
		t.Fatal("missing outer word ignored")
	}
	c.retained.pool.words[-2] = new(big.Int)
	if c.retainedNeedsCapture(big.NewInt(1000000), true) {
		t.Fatal("new window not recognized")
	}
}

// TestInitializeRetainedPoolCapturesCompleteWindow verifies initialization includes distant ticks.
//
// Version:
//   - 2026-09-12: Verify large reference amounts do not trigger trial quotes.
//   - 2026-09-11: Added.
func TestInitializeRetainedPoolCapturesCompleteWindow(t *testing.T) {
	c, plain := newTestCache(t)
	plain.crossed = true
	f := &feeCaptureFake{oracleCaptureFake: &oracleCaptureFake{stateFake: plain}, module: common.HexToAddress("0x10"), factory: c.factory, pool: c.pool}
	c.rpc = &windowCaptureFake{f}
	s, err := c.initializeRetainedPool(context.Background(), new(big.Int).Lsh(big.NewInt(1), 200), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.pool.words) != 3 || len(s.pool.ticks) != 3 || s.pool.ticks[15420] == nil {
		t.Fatal("initialization omitted window tick details")
	}
}

type windowCaptureFake struct{ *feeCaptureFake }

// LatestHeader returns the oracle fixture baseline.
//
// Version:
//   - 2026-09-11: Added.
func (f *windowCaptureFake) LatestHeader(ctx context.Context) (evm.BlockHeader, error) {
	h, err := f.stateFake.LatestHeader(ctx)
	h.Timestamp = 100
	return h, err
}

// ReadContracts dispatches core, bitmap, tick and oracle batches through their ABI fixtures.
//
// Version:
//   - 2026-09-11: Added.
func (f *windowCaptureFake) ReadContracts(ctx context.Context, target common.Address, calls [][]byte, block uint64) ([][]byte, []error, error) {
	if string(calls[0][:4]) == string(crypto.Keccak256([]byte("observations(uint256)"))[:4]) {
		return f.oracleCaptureFake.ReadContracts(ctx, target, calls, block)
	}
	values, failures := make([][]byte, len(calls)), make([]error, len(calls))
	for i, call := range calls {
		values[i], failures[i] = f.CallContract(ctx, ethereum.CallMsg{To: &target, Data: call}, new(big.Int).SetUint64(block))
	}
	return values, failures, nil
}
