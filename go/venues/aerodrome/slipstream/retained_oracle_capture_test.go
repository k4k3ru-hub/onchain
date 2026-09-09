package slipstream

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type oracleCaptureFake struct {
	*stateFake
	batches []int
	fail    error
}

// CallContract returns the pinned oracle slot configuration.
//
// Version:
//   - 2026-09-09: Added.
func (f *oracleCaptureFake) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if block == nil || block.Uint64() != 100 {
		f.t.Fatal("unpinned core")
	}
	data, err := f.stateFake.CallContract(ctx, msg, block)
	if err != nil {
		return nil, err
	}
	if len(data) == 192 {
		big.NewInt(129).FillBytes(data[96:128])
		big.NewInt(129).FillBytes(data[128:160])
	}
	return data, nil
}

// ReadContracts returns complete observation batches or an injected element failure.
//
// Version:
//   - 2026-09-09: Added.
func (f *oracleCaptureFake) ReadContracts(_ context.Context, _ common.Address, calls [][]byte, block uint64) ([][]byte, []error, error) {
	if block != 100 {
		f.t.Fatal("unpinned batch")
	}
	f.batches = append(f.batches, len(calls))
	values := make([][]byte, len(calls))
	failures := make([]error, len(calls))
	for i := range calls {
		values[i] = make([]byte, 128)
		big.NewInt(100).FillBytes(values[i][:32])
		big.NewInt(1).FillBytes(values[i][96:])
	}
	failures[0] = f.fail
	return values, failures, nil
}

// TestCaptureRetainedOraclePinsAndBatches verifies bounded batches and error propagation.
//
// Version:
//   - 2026-09-09: Added.
func TestCaptureRetainedOraclePinsAndBatches(t *testing.T) {
	f := &oracleCaptureFake{stateFake: &stateFake{t: t}}
	c := &StateCache{rpc: f, pool: common.HexToAddress("0x1")}
	s := &poolSnapshot{header: evm.BlockHeader{Number: 100}, price: power2(96)}
	o, err := c.captureRetainedOracle(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.slots) != 129 || len(f.batches) != 3 || f.batches[0] != 64 || f.batches[1] != 64 || f.batches[2] != 1 {
		t.Fatalf("batch shape=%v", f.batches)
	}
	sentinel := errors.New("missing observation")
	f.fail = sentinel
	if _, err := c.captureRetainedOracle(context.Background(), s); !errors.Is(err, sentinel) {
		t.Fatalf("lost failure: %v", err)
	}
	if _, err := decodeTickObservation(nil); err == nil {
		t.Fatal("missing data accepted")
	}
	data := make([]byte, 128)
	data[127] = 2
	if _, err := decodeTickObservation(data); err == nil {
		t.Fatal("noncanonical bool accepted")
	}
}
