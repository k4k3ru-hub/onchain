package quotestate

import (
	"errors"
	"fmt"
	"testing"
)

// TestIsStateChangePreservesJoinedTransportFailures verifies quote-state recovery invariants.
//
// Version:
//   - 2026-09-09: Added.
func TestIsStateChangePreservesJoinedTransportFailures(t *testing.T) {
	wrapped := fmt.Errorf("failed to quote: %w", ErrStateChanged)
	if !IsStateChange(wrapped) || !IsStateChange(errors.Join(wrapped, ErrStateChanged)) {
		t.Fatal("state change not classified")
	}
	if IsStateChange(errors.Join(wrapped, errors.New("rate limited"))) || IsStateChange(nil) {
		t.Fatal("unsafe retry classification")
	}
}
