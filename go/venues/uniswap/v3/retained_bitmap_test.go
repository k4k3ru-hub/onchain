package v3

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

type bitmapBatchFake struct {
	*stateFake
	batches   int
	failure   error
	transport bool
	malformed bool
}

// ReadContracts supplies pinned bitmap batches for regression tests.
//
// Version:
//   - 2026-09-11: Added.
func (f *bitmapBatchFake) ReadContracts(ctx context.Context, target common.Address, calls [][]byte, block uint64) ([][]byte, []error, error) {
	f.batches++
	if block != 100 || len(calls) != 3 {
		f.t.Fatalf("block=%d count=%d", block, len(calls))
	}
	for i, call := range calls {
		want := big.NewInt(int64(i - 1))
		if want.Sign() < 0 {
			want.Add(want, power2(256))
		}
		if new(big.Int).SetBytes(call[4:]).Cmp(want) != 0 {
			f.t.Fatal("wrong word encoding")
		}
	}
	if f.transport {
		return nil, nil, f.failure
	}
	values, failures := make([][]byte, len(calls)), make([]error, len(calls))
	for i, call := range calls {
		var err error
		values[i], err = f.stateFake.CallContract(ctx, ethereum.CallMsg{To: &target, Data: call}, new(big.Int).SetUint64(block))
		if err != nil {
			return nil, nil, err
		}
	}
	failures[1] = f.failure
	if f.malformed {
		values[1] = nil
	}
	return values, failures, nil
}

// TestRetainedBitmapBatch verifies parity, pinned encoding and atomic failure.
//
// Version:
//   - 2026-09-11: Added.
func TestRetainedBitmapBatch(t *testing.T) {
	ctx := context.Background()
	plain, _ := newTestCache(t)
	want, err := plain.captureRetainedWindow(ctx, big.NewInt(1000000), true, 0, common.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "element", "transport", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			c, f := newTestCache(t)
			batch := &bitmapBatchFake{stateFake: f}
			injected := errors.New("injected batch failure")
			if mode == "element" || mode == "transport" {
				batch.failure = injected
			}
			batch.transport = mode == "transport"
			batch.malformed = mode == "malformed"
			c.rpc = batch
			got, err := c.captureRetainedWindow(ctx, big.NewInt(1000000), true, 0, common.Hash{})
			if batch.batches != 1 {
				t.Fatalf("batches=%d", batch.batches)
			}
			if mode == "success" {
				if err != nil || !reflect.DeepEqual(want.words, got.words) || f.calls != 6 {
					t.Fatalf("result=%v calls=%d", err, f.calls)
				}
			} else {
				if err == nil || got != nil {
					t.Fatal("partial window accepted")
				}
				if batch.failure != nil && !errors.Is(err, injected) {
					t.Fatalf("lost cause: %v", err)
				}
				expected := 6
				if batch.transport {
					expected = 3
				}
				if f.calls != expected {
					t.Fatalf("unexpected fallback calls=%d", f.calls)
				}
			}
		})
	}
}

// TestBitmapBatchBudget verifies logical reads still consume the acquisition cap.
//
// Version:
//   - 2026-09-11: Added.
func TestBitmapBatchBudget(t *testing.T) {
	c, f := newTestCache(t)
	batch := &bitmapBatchFake{stateFake: f}
	c.rpc = batch
	s := &poolSnapshot{words: make(map[int32]*big.Int)}
	budget := 2
	err := c.readBitmapWindow(context.Background(), s, -1, 1, &budget)
	if !errors.Is(err, errStateReadBudget) || batch.batches != 0 || len(s.words) != 0 {
		t.Fatalf("err=%v batches=%d", err, batch.batches)
	}
}
