// Package tickbatch stages independent tick reads before publishing results.
package tickbatch

import (
	"context"
	"fmt"
	"sync"
)

// Read fetches batches with at most two workers when the reader explicitly supports concurrency.
// All results retain input order; failure cancels outstanding work and discards results.
//
// Version:
//   - 2026-10-23: Added.
func Read[T any](ctx context.Context, reader any, count int, fetch func(context.Context, int) (T, error)) ([]T, error) {
	workers := 1
	if r, ok := reader.(interface{ TickReadConcurrency() int }); ok && r.TickReadConcurrency() >= 2 {
		workers = 2
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]T, count)
	var mu sync.Mutex
	var first error
	next := 0
	var wg sync.WaitGroup
	for w := 0; w < min(workers, count); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				if first != nil || child.Err() != nil || next >= count {
					mu.Unlock()
					return
				}
				i := next
				next++
				mu.Unlock()
				value, err := fetch(child, i)
				if err != nil {
					mu.Lock()
					if first == nil {
						first = err
						cancel()
					}
					mu.Unlock()
					return
				}
				results[i] = value
			}
		}()
	}
	wg.Wait()
	if first != nil {
		return nil, fmt.Errorf("failed to read tick batches: %w", first)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("failed to read tick batches: %w", err)
	}
	return results, nil
}
