package solana

import (
	"errors"
	"fmt"
	"slices"
)

var (
	ErrQuoteArrayLimit    = errors.New("failed to complete quote: array acquisition limit reached")
	ErrQuoteSnapshotLimit = errors.New("failed to complete quote: snapshot attempt limit reached")
	ErrQuoteArrayRange    = errors.New("failed to complete quote: supported array search range exhausted")
)

type QuoteArrayRequiredError struct{ Address Address }

// Error describes an array missing from the current snapshot.
//
// Version:
//   - 2026-09-05: Added.
func (e *QuoteArrayRequiredError) Error() string {
	return "failed to read quote array: array is outside snapshot"
}

// NormalizeQuoteArrayCounts applies defaults and validates total array budgets.
//
// Returns:
//   - Initial and maximum counts, defaulting to 5 and 16 respectively.
//   - Validation error when counts are negative, initial exceeds maximum, or maximum exceeds 32.
//
// Version:
//   - 2026-09-05: Added.
func NormalizeQuoteArrayCounts(initial, maximum int) (int, int, error) {
	if initial == 0 {
		initial = 5
	}
	if maximum == 0 {
		maximum = 16
	}
	if initial < 1 || maximum < 1 || initial > maximum || maximum > 32 {
		return 0, 0, fmt.Errorf("failed to validate quote array counts: counts=out_of_range initial_array_count=%d max_array_count=%d min_value=1 max_value=32", initial, maximum)
	}
	return initial, maximum, nil
}

// SelectQuoteArrays selects nearby arrays and retains previously requested relevant arrays.
//
// Parameters:
//   - candidates: unique arrays ordered by proximity, interleaving requested directions.
//   - previous: arrays selected during this quote, not data from previous snapshots.
//   - initial: initial total array budget.
//
// Version:
//   - 2026-09-05: Added.
func SelectQuoteArrays(candidates, previous []Address, initial int) []Address {
	selected := append([]Address(nil), candidates[:min(max(initial, 0), len(candidates))]...)
	for _, address := range previous {
		if slices.Contains(candidates, address) && !slices.Contains(selected, address) {
			selected = append(selected, address)
		}
	}
	return selected
}
