package slipstream

import (
	"context"
	"fmt"
	"math/big"
)

// QuoteSnapshot owns detached calculation inputs without network dependencies.
// Its inputs are private and each calculation clones mutable arithmetic state.
type QuoteSnapshot struct{ cache StateCache }

// QuotePair calculates against these frozen inputs without refreshing live state.
//
// Version:
//   - 2026-09-09: Added.
func (s *QuoteSnapshot) QuotePair(ctx context.Context, amount *big.Int, baseIsToken0 bool) (LocalPair, error) {
	if s == nil {
		return LocalPair{}, fmt.Errorf("failed to quote frozen state: snapshot=null")
	}
	return s.cache.QuoteRetainedPair(ctx, amount, baseIsToken0)
}

// CaptureQuoteSnapshot detaches the current inputs; nil means unavailable.
//
// Version:
//   - 2026-09-09: Added.
func (c *StateCache) CaptureQuoteSnapshot() *QuoteSnapshot {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.captureQuoteSnapshotLocked()
}
func (c *StateCache) captureQuoteSnapshotLocked() *QuoteSnapshot {
	if c.retained == nil {
		return nil
	}
	return &QuoteSnapshot{cache: StateCache{retained: cloneRetainedPool(c.retained)}}
}

// SetQuoteSnapshotObserver installs the state consumer and immediately publishes current inputs.
// The callback runs under the state lock, must be short, and must not call back into the cache.
// A nil snapshot withdraws invalidated state. Pass nil to remove the observer.
//
// Version:
//   - 2026-09-09: Added.
func (c *StateCache) SetQuoteSnapshotObserver(observer func(*QuoteSnapshot)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.quoteSnapshotObserver = observer
	c.publishQuoteSnapshotLocked()
}
func (c *StateCache) publishQuoteSnapshotLocked() {
	if c.quoteSnapshotObserver != nil {
		c.quoteSnapshotObserver(c.captureQuoteSnapshotLocked())
	}
}
