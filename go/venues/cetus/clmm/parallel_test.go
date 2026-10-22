package clmm

import (
	"context"
	"encoding/json"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"sync"
	"testing"
)

type parallelWindow struct {
	*windowReaderFake
	mu sync.Mutex
}

// TickReadConcurrency enables the concurrent fixture.
// Version:
//   - 2026-10-23: Added.
func (*parallelWindow) TickReadConcurrency() int { return 2 }

// DynamicUint64ValuesAtCheckpoint protects fixture counters and verifies the pinned checkpoint.
// Version:
//   - 2026-10-23: Added.
func (r *parallelWindow) DynamicUint64ValuesAtCheckpoint(ctx context.Context, p sui.Address, c sui.CheckpointSequenceNumber, k []uint64) ([]json.RawMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.windowReaderFake.DynamicUint64ValuesAtCheckpoint(ctx, p, c, k)
}

// TestParallelCapturePinnedAndAtomic verifies bounded reads and complete results.
//
// Version:
//   - 2026-10-23: Added.
func TestParallelCapturePinnedAndAtomic(t *testing.T) {
	_, r, _ := windowFixture(t)
	reader := &parallelWindow{windowReaderFake: r}
	snapshot, err := captureState(context.Background(), reader, r.obj, r.cp)
	if err != nil || snapshot == nil || r.calls != 16 || len(r.keys) != 768 {
		t.Fatalf("capture err=%v calls=%d keys=%d", err, r.calls, len(r.keys))
	}
	r.calls = 0
	r.failAt = 3
	snapshot, err = captureState(context.Background(), reader, r.obj, r.cp)
	if err == nil || snapshot != nil {
		t.Fatal("partial result published")
	}
}
