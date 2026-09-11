package v4

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type tickWindowBatchFake struct {
	*stateFake
	poolID    common.Hash
	target    common.Address
	sizes     []int
	failure   error
	malformed bool
	cancel    context.CancelFunc
}

// ReadContracts returns deterministic signed tick data at the snapshot block.
//
// Version:
//   - 2026-09-11: Added.
func (f *tickWindowBatchFake) ReadContracts(ctx context.Context, target common.Address, calls [][]byte, block uint64) ([][]byte, []error, error) {
	if target == (common.Address{}) || block != 100 {
		f.t.Fatal("unpinned tick batch")
	}
	f.sizes = append(f.sizes, len(calls))
	values, failures := make([][]byte, len(calls)), make([]error, len(calls))
	if target != f.target {
		f.t.Fatal("wrong StateView target")
	}
	for _, call := range calls {
		if len(call) != 68 || common.BytesToHash(call[4:36]) != f.poolID {
			f.t.Fatal("wrong pool ID ABI")
		}
	}
	for i, call := range calls {
		arg := new(big.Int).SetBytes(call[36:])
		if arg.Bit(255) != 0 {
			arg.Sub(arg, power2(256))
		}
		if arg.Int64() < -256 || arg.Int64() > 511 {
			f.t.Fatal("unexpected tick")
		}
		values[i] = make([]byte, 64)
		big.NewInt(10).FillBytes(values[i][:32])
		new(big.Int).Sub(power2(256), big.NewInt(3)).FillBytes(values[i][32:64])
	}
	if len(f.sizes) == 2 {
		failures[0] = f.failure
		if f.malformed {
			values[0] = nil
		}
	}
	if f.cancel != nil {
		f.cancel()
	}
	return values, failures, nil
}

func tickWindowFixture() *poolSnapshot {
	s := &poolSnapshot{spacing: 1, price: power2(96), liquidity: integer("1000000000000000000"), header: evm.BlockHeader{Number: 100}, words: make(map[int32]*big.Int), ticks: make(map[int32]*big.Int), gross: make(map[int32]*big.Int)}
	for w := int32(-1); w <= 1; w++ {
		s.words[w] = new(big.Int)
	}
	for i := 0; i < 40; i++ {
		s.words[-1].SetBit(s.words[-1], i, 1)
	}
	s.words[1].SetBit(s.words[1], 255, 1)
	return s
}

// TestCompleteWindowTicks verifies distant ticks, signed values, chunking and atomic failure.
//
// Version:
//   - 2026-09-11: Added.
func TestCompleteWindowTicks(t *testing.T) {
	for _, mode := range []string{"success", "failure", "malformed", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			c, f := newTestCache(t)
			b := &tickWindowBatchFake{stateFake: f, poolID: c.poolID, target: c.stateView}
			injected := errors.New("injected tick failure")
			if mode == "failure" {
				b.failure = injected
			}
			b.malformed = mode == "malformed"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				b.cancel = cancel
			}
			c.rpc = b
			s := tickWindowFixture()
			err := c.readWindowTicks(ctx, s, -1, 1)
			if mode == "success" {
				if err != nil || len(s.ticks) != 41 || !reflect.DeepEqual(b.sizes, []int{32, 9}) {
					t.Fatalf("err=%v ticks=%d batches=%v", err, len(s.ticks), b.sizes)
				}
				if s.ticks[511].Int64() != -3 || s.gross[-256].Int64() != 10 {
					t.Fatal("wrong tick fields")
				}
			} else {
				if err == nil || len(s.ticks) != 0 || len(s.gross) != 0 {
					t.Fatalf("partial ticks installed: %v", err)
				}
				if mode == "failure" && !errors.Is(err, injected) {
					t.Fatal("lost cause")
				}
				if mode == "cancel" && (!errors.Is(err, context.Canceled) || len(b.sizes) != 1) {
					t.Fatal("cancellation ignored")
				}
			}
		})
	}
}

// TestMissingDistantTickRequestsCapture verifies coverage checking beyond the current quote.
//
// Version:
//   - 2026-09-11: Added.
func TestMissingDistantTickRequestsCapture(t *testing.T) {
	c, _ := newTestCache(t)
	c.retained = tickWindowFixture()
	if !c.retainedNeedsCapture(big.NewInt(1), true) {
		t.Fatal("missing distant ticks accepted")
	}
}

// TestWindowTicksSequentialFallback verifies complete acquisition without batch support.
//
// Version:
//   - 2026-09-11: Added.
func TestWindowTicksSequentialFallback(t *testing.T) {
	c, f := newTestCache(t)
	s := tickWindowFixture()
	if err := c.readWindowTicks(context.Background(), s, -1, 1); err != nil {
		t.Fatal(err)
	}
	if len(s.ticks) != 41 || f.calls != 41 {
		t.Fatalf("ticks=%d calls=%d", len(s.ticks), f.calls)
	}
}

// TestCapturedWindowQuotesAfterMovement verifies distant prefetched ticks serve local quotes.
//
// Version:
//   - 2026-09-11: Added.
func TestCapturedWindowQuotesAfterMovement(t *testing.T) {
	c, f := newTestCache(t)
	f.crossed = true
	s, err := c.captureRetainedWindow(context.Background(), new(big.Int).Lsh(big.NewInt(1), 200), true, 0, common.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.ticks) != 3 {
		t.Fatalf("ticks=%d", len(s.ticks))
	}
	// Move near a tick in the farthest positive word, beyond the original quote.
	s.tick = 15419
	s.price = sqrtAtTick(s.tick)
	calls := f.calls
	budget := 0
	if _, err := c.quote(context.Background(), s, integer("1000000000000000"), false, true, &budget); err != nil {
		t.Fatal(err)
	}
	if f.calls != calls {
		t.Fatal("local quote fetched state")
	}
	delete(s.ticks, 15420)
	budget = 0
	if _, err := c.quote(context.Background(), s, integer("1000000000000000"), false, true, &budget); !errors.Is(err, errStateReadBudget) {
		t.Fatalf("missing tick did not block quote: %v", err)
	}
}
