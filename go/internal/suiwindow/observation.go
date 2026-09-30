package suiwindow

import (
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"time"
)

// ObserveProgress advances coverage only on a fully covered checkpoint watermark.
// A repeated checkpoint or a live socket alone does not establish new coverage.
// Call after applying and validating every preceding transaction in stream order.
//
// Version:
//   - 2026-10-01: Added.
func ObserveProgress(observation *quotestate.Observation, checkpoint *uint64, n *sui.TransactionNotification, baseline uint64, availableAt time.Time) error {
	if observation == nil || checkpoint == nil {
		return fmt.Errorf("failed to observe sui progress: dependency=null")
	}
	if n == nil {
		return nil
	}
	if n.ObjectChangesError != nil {
		return fmt.Errorf("failed to observe sui progress: %w", n.ObjectChangesError)
	}
	if n.Effects != nil && n.Effects.Timestamp != nil {
		observation.SourceTime = *n.Effects.Timestamp
	}
	if !observation.Active || n.Watermark.Checkpoint == nil {
		return nil
	}
	sequence := n.Watermark.Checkpoint.Uint64()
	if sequence < *checkpoint {
		return fmt.Errorf("failed to observe sui progress: checkpoint=regressed")
	}
	if sequence < baseline || sequence == *checkpoint {
		return nil
	}
	*checkpoint = sequence
	observation.ConfirmedAt = availableAt
	return nil
}
