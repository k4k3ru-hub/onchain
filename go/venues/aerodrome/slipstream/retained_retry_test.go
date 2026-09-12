package slipstream

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type retryWindowRPC struct {
	*windowCaptureFake
	header                   evm.BlockHeader
	latestCalls, bitmapCalls int
	batches                  [][]int64
	blocks                   []uint64
	failure                  error
	mode                     string
	verifyFailure            bool
}

// LatestHeader returns the controlled baseline and counts new captures.
//
// Version:
//   - 2026-09-12: Added.
func (r *retryWindowRPC) LatestHeader(context.Context) (evm.BlockHeader, error) {
	r.latestCalls++
	r.header.Timestamp = 100
	return r.header, nil
}

// HeaderByNumber verifies the capture without advancing its baseline.
//
// Version:
//   - 2026-09-12: Added.
func (r *retryWindowRPC) HeaderByNumber(_ context.Context, number uint64) (evm.BlockHeader, error) {
	if r.verifyFailure {
		r.verifyFailure = false
		return evm.BlockHeader{}, r.failure
	}
	return evm.BlockHeader{Number: number, Hash: r.header.Hash, Timestamp: 100}, nil
}

// ReadContracts serves a two-batch window and injects a failure in its second batch.
//
// Version:
//   - 2026-09-12: Added.
func (r *retryWindowRPC) ReadContracts(ctx context.Context, target common.Address, calls [][]byte, block uint64) ([][]byte, []error, error) {
	values, failures := make([][]byte, len(calls)), make([]error, len(calls))
	if string(calls[0][:4]) == string(crypto.Keccak256([]byte("tickBitmap(int16)"))[:4]) {
		r.bitmapCalls++
		fixture := tickWindowFixture()
		for i, call := range calls {
			word := int32(new(big.Int).SetBytes(call[4:]).Int64())
			values[i] = fixture.words[word].FillBytes(make([]byte, 32))
		}
		return values, failures, nil
	}
	if string(calls[0][:4]) != string(crypto.Keccak256([]byte("ticks(int24)"))[:4]) {
		return r.windowCaptureFake.ReadContracts(ctx, target, calls, block)
	}
	var ticks []int64
	for i, call := range calls {
		ticks = append(ticks, new(big.Int).SetBytes(call[4:]).Int64())
		value, err := r.stateFake.CallContract(ctx, ethereum.CallMsg{To: &target, Data: call}, new(big.Int).SetUint64(block))
		if err != nil {
			return nil, nil, err
		}
		values[i] = value
	}
	r.batches = append(r.batches, ticks)
	r.blocks = append(r.blocks, block)
	if len(r.batches) == 2 {
		switch r.mode {
		case "transport":
			return nil, nil, r.failure
		case "element":
			failures[len(failures)-1] = r.failure
		case "malformed":
			values[len(values)-1] = nil
		case "deadline":
			<-ctx.Done()
			return nil, nil, ctx.Err()
		case "verification":
			r.verifyFailure = true
		}
	}
	return values, failures, nil
}

func newRetryWindowRPC(t *testing.T) (*StateCache, *retryWindowRPC) {
	t.Helper()
	c, fake := newTestCache(t)
	f := &feeCaptureFake{oracleCaptureFake: &oracleCaptureFake{stateFake: fake}, module: common.HexToAddress("10"), factory: c.factory, pool: c.pool}
	r := &retryWindowRPC{windowCaptureFake: &windowCaptureFake{f}, header: evm.BlockHeader{Number: 100, Hash: common.HexToHash("01")}, failure: errors.New("injected batch failure")}
	c.rpc = r
	return c, r
}

// TestRetainedCaptureResumesBatches verifies private progress survives a failed attempt.
//
// Version:
//   - 2026-09-12: Added.
func TestRetainedCaptureResumesBatches(t *testing.T) {
	for _, mode := range []string{"transport", "element", "malformed", "deadline", "verification"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c, r := newRetryWindowRPC(t)
				r.mode = mode
				progress := &retainedWindowCapture{}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				got, err := c.resumeRetainedPool(ctx, progress)
				if err == nil || got != nil || c.retained != nil {
					t.Fatalf("published failed capture: %v", err)
				}
				if mode == "transport" || mode == "element" || mode == "verification" {
					if !errors.Is(err, r.failure) {
						t.Fatalf("lost cause: %v", err)
					}
				}
				if mode != "verification" {
					for _, field := range []string{"pool_id=", "block_number=100", "tick_count=41", "completed_tick_count=32", "batch_index=2", "batch_count=2", "batch_size=9", "elapsed_seconds=", "batch_elapsed_seconds="} {
						if !strings.Contains(err.Error(), field) {
							t.Fatalf("missing %s: %v", field, err)
						}
					}
					if len(progress.snapshot.ticks) != 0 || progress.ticks.next != 32 {
						t.Fatal("partial batch published or advanced")
					}
				}
				if mode == "deadline" {
					if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "elapsed_seconds=30.000") {
						t.Fatal(err)
					}
				}
				// Latest has advanced, but completed bitmap and tick reads belong to 100.
				r.header.Number = 101
				retryCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
				defer stop()
				got, err = c.resumeRetainedPool(retryCtx, progress)
				if err != nil || got == nil {
					t.Fatal(err)
				}
				if got.pool.header.Number != 100 || len(got.pool.ticks) != 41 || r.latestCalls != 1 || r.bitmapCalls != 1 {
					t.Fatal("capture restarted or mixed blocks")
				}
				if mode == "verification" {
					if len(r.batches) != 2 {
						t.Fatal("verification failure repeated ticks")
					}
				} else if len(r.batches) != 3 || !reflect.DeepEqual(r.batches[1], r.batches[2]) {
					t.Fatalf("wrong retry batches: %v", r.batches)
				}
				for _, block := range r.blocks {
					if block != 100 {
						t.Fatal("unpinned retry")
					}
				}
			})
		})
	}
}

// TestRetainedCaptureDiscardsChangedHash prevents resumed reads from mixing block branches.
//
// Version:
//   - 2026-09-12: Added.
func TestRetainedCaptureDiscardsChangedHash(t *testing.T) {
	c, r := newRetryWindowRPC(t)
	r.mode = "transport"
	progress := &retainedWindowCapture{}
	if _, err := c.resumeRetainedPool(context.Background(), progress); err == nil {
		t.Fatal("missing failure")
	}
	r.header.Hash = common.HexToHash("02")
	if _, err := c.resumeRetainedPool(context.Background(), progress); err == nil || !strings.Contains(err.Error(), "block_hash=mismatch") {
		t.Fatal(err)
	}
	if progress.snapshot != nil || progress.ticks.staged != nil || len(r.batches) != 2 {
		t.Fatal("reused conflicting progress")
	}
	got, err := c.resumeRetainedPool(context.Background(), progress)
	if err != nil || got.pool.header.Hash != r.header.Hash || len(r.batches) != 4 || len(r.batches[2]) != 32 {
		t.Fatalf("did not restart complete capture: %v", err)
	}
}
