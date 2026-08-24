package solana

import "testing"

func TestCommitment(t *testing.T) {
	for _, commitment := range []Commitment{
		CommitmentProcessed,
		CommitmentConfirmed,
		CommitmentFinalized,
	} {
		if !commitment.IsValid() {
			t.Errorf("Commitment(%q).IsValid() = false, want true", commitment)
		}
		if err := commitment.Validate(); err != nil {
			t.Errorf("Commitment(%q).Validate() error = %v", commitment, err)
		}
		if got := commitment.String(); got != string(commitment) {
			t.Errorf("Commitment(%q).String() = %q", commitment, got)
		}
	}
}

func TestCommitmentRejectsInvalidValue(t *testing.T) {
	for _, commitment := range []Commitment{"", "invalid"} {
		if commitment.IsValid() {
			t.Errorf("Commitment(%q).IsValid() = true, want false", commitment)
		}
		if err := commitment.Validate(); err == nil {
			t.Errorf("Commitment(%q).Validate() error = nil, want error", commitment)
		}
	}
}
