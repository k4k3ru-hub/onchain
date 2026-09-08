package slipstream

import (
	"context"
	"errors"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"testing"
)

type batchStateFake struct {
	*stateFake
	batches int
	fail    error
}

func (f *batchStateFake) ReadContracts(ctx context.Context, target common.Address, data [][]byte, block uint64) ([][]byte, []error, error) {
	f.batches++
	out := make([][]byte, len(data))
	errs := make([]error, len(data))
	for i, d := range data {
		out[i], errs[i] = f.CallContract(ctx, ethereum.CallMsg{To: &target, Data: d}, new(big.Int).SetUint64(block))
	}
	if f.fail != nil {
		errs[1] = f.fail
	}
	return out, errs, nil
}

// TestCoreStateBatchParityAndFailure verifies or implements the injected batch test boundary.
//
// Version:
//   - 2026-09-08: Added.
func TestCoreStateBatchParityAndFailure(t *testing.T) {
	sequential, _ := newTestCache(t)
	want, err := sequential.QuotePair(context.Background(), big.NewInt(1000000), true)
	if err != nil {
		t.Fatal(err)
	}
	cache, f := newTestCache(t)
	batch := &batchStateFake{stateFake: f}
	cache.rpc = batch
	got, err := cache.QuotePair(context.Background(), big.NewInt(1000000), true)
	if err != nil {
		t.Fatal(err)
	}
	if batch.batches != 1 || want.BidAmountOut.Cmp(got.BidAmountOut) != 0 || want.AskAmountIn.Cmp(got.AskAmountIn) != 0 || want.FeePPM != got.FeePPM {
		t.Fatal("batch changed quote")
	}
	batch.fail = errors.New("injected")
	before := f.calls
	_, err = cache.QuotePair(context.Background(), big.NewInt(1000000), true)
	if !errors.Is(err, batch.fail) || cache.snapshot != nil || f.calls-before != 3 {
		t.Fatalf("partial batch accepted or retried: %v", err)
	}
}
