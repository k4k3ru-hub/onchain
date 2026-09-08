package sui

import (
	"context"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
)

type checkpointHeadReader interface {
	LatestCheckpoint(context.Context) (Checkpoint, error)
}
type checkpointNumberReader interface {
	CheckpointBySequenceNumber(context.Context, CheckpointSequenceNumber) (Checkpoint, error)
}

// QuoteCheckpoint obtains a head at least as recent as the quote-owned notification floor.
// A lagging latest response uses a numbered checkpoint when the reader supports it.
// Transport failures remain wrapped and do not become state-change retries.
//
// Version:
//   - 2026-09-09: Added.
func QuoteCheckpoint(ctx context.Context, reader checkpointHeadReader, floor CheckpointSequenceNumber) (Checkpoint, error) {
	head, err := reader.LatestCheckpoint(ctx)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get quote checkpoint: %w", err)
	}
	if head.SequenceNumber >= floor {
		return head, nil
	}
	numbered, ok := reader.(checkpointNumberReader)
	if !ok {
		return Checkpoint{}, fmt.Errorf("failed to get quote checkpoint: %w: checkpoint=behind observed_checkpoint=%d required_checkpoint=%d", quotestate.ErrStateChanged, head.SequenceNumber, floor)
	}
	head, err = numbered.CheckpointBySequenceNumber(ctx, floor)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get observed quote checkpoint: %w", err)
	}
	if head.SequenceNumber != floor {
		return Checkpoint{}, fmt.Errorf("failed to get observed quote checkpoint: sequence_number=mismatch")
	}
	return head, nil
}
