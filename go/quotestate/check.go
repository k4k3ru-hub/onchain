// Package quotestate describes verified inputs for event-driven local quotes.
package quotestate

import "time"

// Check identifies calculation inputs, separately from their verification position.
// Key is opaque and is only comparable within the same cache instance.
type Check struct {
	Revision  uint64
	Key       string
	Position  uint64
	CheckedAt time.Time
}
