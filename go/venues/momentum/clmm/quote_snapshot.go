package clmm

import (
	"context"
	"fmt"
)

// QuoteSnapshot owns detached calculation inputs without network dependencies.
// Its inputs are private and each calculation clones mutable arithmetic state.
type QuoteSnapshot struct{ cache StateCache }

// QuotePair calculates against these frozen inputs without refreshing live state.
//
// Version:
//   - 2026-09-09: Added.
func (s *QuoteSnapshot) QuotePair(ctx context.Context, p QuotePairParams) (QuotePairResult, error) {
	if s == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote frozen state: snapshot=null")
	}
	return s.cache.QuoteRetainedPair(ctx, p)
}

// CaptureQuoteSnapshot detaches the current inputs; nil means unavailable.
//
// Version:
//   - 2026-09-09: Added.
func (c *StateCache) CaptureQuoteSnapshot() *QuoteSnapshot {
	if c == nil {
		return nil
	}
	c.retainedMu.Lock()
	defer c.retainedMu.Unlock()
	return c.captureQuoteSnapshotLocked()
}
func (c *StateCache) captureQuoteSnapshotLocked() *QuoteSnapshot {
	if c.retained == nil {
		return nil
	}
	return &QuoteSnapshot{cache: StateCache{pool: c.pool, retained: cloneRetainedState(c.retained), retainedHead: c.retainedHead}}
}

// SetQuoteSnapshotObserver installs the state consumer and immediately publishes current inputs.
// The callback runs under the state lock, must be short, and must not call back into the cache.
// A nil snapshot withdraws invalidated state. Pass nil to remove the observer.
//
// Version:
//   - 2026-09-09: Added.
func (c *StateCache) SetQuoteSnapshotObserver(observer func(*QuoteSnapshot)) {
	c.retainedMu.Lock()
	defer c.retainedMu.Unlock()
	c.quoteSnapshotObserver = observer
	c.publishQuoteSnapshotLocked()
}
func (c *StateCache) publishQuoteSnapshotLocked() {
	if c.quoteSnapshotObserver != nil {
		c.quoteSnapshotObserver(c.captureQuoteSnapshotLocked())
	}
}
