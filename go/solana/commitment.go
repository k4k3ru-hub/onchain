package solana

import "fmt"

type Commitment string

const (
	CommitmentProcessed Commitment = "processed"
	CommitmentConfirmed Commitment = "confirmed"
	CommitmentFinalized Commitment = "finalized"
)

// IsValid reports whether the Solana commitment is supported.
//
// Returns:
//   - True for processed, confirmed, or finalized.
//
// Version:
//   - 2026-08-22: Added.
func (c Commitment) IsValid() bool {
	switch c {
	case CommitmentProcessed, CommitmentConfirmed, CommitmentFinalized:
		return true
	default:
		return false
	}
}

// Validate validates a Solana commitment.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-22: Added.
func (c Commitment) Validate() error {
	if c == "" {
		return fmt.Errorf("failed to validate solana commitment: commitment=empty")
	}
	if !c.IsValid() {
		return fmt.Errorf("failed to validate solana commitment: commitment=invalid")
	}
	return nil
}

// String returns the Solana commitment string.
//
// Returns:
//   - Solana commitment string.
//
// Version:
//   - 2026-08-22: Added.
func (c Commitment) String() string {
	return string(c)
}
