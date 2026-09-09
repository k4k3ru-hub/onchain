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
	batches     []int
	fail        error
	cardinality int
	now         uint32
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
		cardinality := f.cardinality
		if cardinality == 0 {
			cardinality = 129
		}
		big.NewInt(int64(cardinality)).FillBytes(data[96:128])
		big.NewInt(int64(cardinality)).FillBytes(data[128:160])
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
		if f.now != 0 {
			index := int(new(big.Int).SetBytes(calls[i][4:]).Int64())
			distance := (f.cardinality - index) % f.cardinality
			big.NewInt(int64(f.now - uint32(distance*2))).FillBytes(values[i][:32])
			big.NewInt(int64(f.now-uint32(distance*2)) * 10).FillBytes(values[i][32:64])
		}
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
	s := &poolSnapshot{header: evm.BlockHeader{Number: 100, Timestamp: 100}, price: power2(96)}
	o, err := c.captureRetainedOracle(context.Background(), s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.slots) != 129 || len(f.batches) != 9 || f.batches[0] != 16 || f.batches[8] != 1 {
		t.Fatalf("batch shape=%v", f.batches)
	}
	sentinel := errors.New("missing observation")
	f.fail = sentinel
	if _, err := c.captureRetainedOracle(context.Background(), s, 2); !errors.Is(err, sentinel) {
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

// TestCaptureRetainedOracleLoadsOnlyFeeWindow verifies RPC reduction without changing ring cardinality.
//
// Version:
//   - 2026-09-09: Added.
func TestCaptureRetainedOracleLoadsOnlyFeeWindow(t *testing.T) {
	f := &oracleCaptureFake{stateFake: &stateFake{t: t}, cardinality: 65535, now: 200000}
	c := &StateCache{rpc: f, pool: common.HexToAddress("0x1")}
	s := &poolSnapshot{header: evm.BlockHeader{Number: 100, Timestamp: uint64(f.now)}, price: power2(96)}
	o, err := c.captureRetainedOracle(context.Background(), s, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.slots) != 65535 || len(f.batches) != 1 || f.batches[0] != 16 {
		t.Fatalf("unexpected capture: ring=%d batches=%v", len(o.slots), f.batches)
	}
	got, ok, err := o.observe(f.now, 19, 10)
	if err != nil || !ok || got != int64(f.now-19)*10 {
		t.Fatalf("interpolation=%d %v %v", got, ok, err)
	}
	if _, _, err := o.observe(f.now, 40, 10); err == nil {
		t.Fatal("unloaded history treated as absent on chain")
	}
	before := o.clone()
	if err := o.write(f.now+2, 10); err != nil {
		t.Fatal(err)
	}
	if !o.known[1] || before.known[1] {
		t.Fatal("coverage write or copy failed")
	}
	got, ok, err = o.observe(f.now+2, 20, 10)
	if err != nil || !ok || got != int64(f.now-18)*10 {
		t.Fatalf("stream continuation=%d %v %v", got, ok, err)
	}
}
