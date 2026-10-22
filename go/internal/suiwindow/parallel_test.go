package suiwindow

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"sync"
	"testing"
)

type parallelWindow struct {
	*windowReader
	mu sync.Mutex
}

// TickReadConcurrency enables the concurrent fixture.
// Version:
//   - 2026-10-23: Added.
func (*parallelWindow) TickReadConcurrency() int { return 2 }

// DynamicValuesByKeysAtCheckpoint protects fixture bookkeeping and validates pinned reads.
// Version:
//   - 2026-10-23: Added.
func (r *parallelWindow) DynamicValuesByKeysAtCheckpoint(ctx context.Context, p sui.Address, c sui.CheckpointSequenceNumber, k []sui.DynamicFieldKey) ([]json.RawMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.windowReader.DynamicValuesByKeysAtCheckpoint(ctx, p, c, k)
}

// TestParallelWindowPinnedAndAtomic verifies bounded reads and complete results.
//
// Version:
//   - 2026-10-23: Added.
func TestParallelWindowPinnedAndAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		boom := errors.New("fixture")
		r := &parallelWindow{windowReader: &windowReader{failure: boom}}
		if fail {
			r.failAt = 3
		}
		var bitmap, ticks sui.Address
		bitmap[31] = 1
		ticks[31] = 2
		words, nets, err := Read(context.Background(), r, 123, bitmap, ticks, "0x1::i32::I32", 0, 1, unsigned)
		if fail {
			if !errors.Is(err, boom) || words != nil || nets != nil {
				t.Fatalf("partial publish: %v", err)
			}
		} else if err != nil || len(nets) != 70 || len(words) != 3 {
			t.Fatalf("capture: %v", err)
		}
	}
}
