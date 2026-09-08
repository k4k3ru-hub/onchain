// Package quotestate describes verified inputs for event-driven local quotes.
package quotestate

import (
	"errors"
	"time"
)

// Check identifies calculation inputs, separately from their verification position.
// Key is opaque and is only comparable within the same cache instance.
type Check struct {
	Revision  uint64
	Key       string
	Position  uint64
	CheckedAt time.Time
}

// ErrStateChanged identifies a coherent-state retry, not a transport failure.
var ErrStateChanged = errors.New("quote state changed")

// IsStateChange reports whether every leaf error is a state-change retry.
// Joined transport failures must retain the transport backoff.
//
// Version:
//   - 2026-09-09: Added.
func IsStateChange(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !IsStateChange(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return IsStateChange(wrapped.Unwrap())
	}
	return err == ErrStateChanged
}
