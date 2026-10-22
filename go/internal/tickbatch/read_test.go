package tickbatch

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type parallelReader struct{}

// TickReadConcurrency enables the parallel fixture.
//
// Version:
//   - 2026-10-23: Added.
func (parallelReader) TickReadConcurrency() int { return 2 }

// TestReadBoundAndOrdering verifies bounded reads and complete results.
//
// Version:
//   - 2026-10-23: Added.
func TestReadBoundAndOrdering(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	entered := make(chan int, 4)
	release := make(chan struct{})
	var active, peak atomic.Int32
	done := make(chan struct{})
	var values []int
	var err error
	go func() {
		defer close(done)
		values, err = Read(ctx, parallelReader{}, 4, func(ctx context.Context, i int) (int, error) {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			entered <- i
			select {
			case <-release:
				return i, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		})
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if active.Load() != 2 {
		t.Fatalf("active=%d", active.Load())
	}
	close(release)
	<-done
	if err != nil || peak.Load() != 2 || len(values) != 4 {
		t.Fatalf("values=%v err=%v peak=%d", values, err, peak.Load())
	}
	for i, v := range values {
		if i != v {
			t.Fatal(values)
		}
	}
}

// TestReadFailureDiscardsAndJoins verifies bounded reads and complete results.
//
// Version:
//   - 2026-10-23: Added.
func TestReadFailureDiscardsAndJoins(t *testing.T) {
	boom := errors.New("fixture")
	entered := make(chan struct{})
	var exited atomic.Bool
	values, err := Read(context.Background(), parallelReader{}, 3, func(ctx context.Context, i int) (int, error) {
		if i == 0 {
			<-entered
			return 0, boom
		}
		close(entered)
		<-ctx.Done()
		exited.Store(true)
		return 0, ctx.Err()
	})
	if values != nil || !errors.Is(err, boom) || !exited.Load() {
		t.Fatalf("values=%v err=%v exited=%t", values, err, exited.Load())
	}
}

// TestReadSequentialAndCanceled verifies bounded reads and complete results.
//
// Version:
//   - 2026-10-23: Added.
func TestReadSequentialAndCanceled(t *testing.T) {
	next := 0
	_, err := Read(context.Background(), struct{}{}, 5, func(_ context.Context, i int) (int, error) {
		if i != next {
			t.Errorf("index=%d next=%d", i, next)
		}
		next++
		return i, nil
	})
	if err != nil || next != 5 {
		t.Fatal(err, next)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	values, err := Read(ctx, parallelReader{}, 2, func(context.Context, int) (int, error) { t.Error("unexpected call"); return 0, nil })
	if values != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(values, err)
	}
}
