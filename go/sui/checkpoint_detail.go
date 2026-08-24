package sui

import (
	"context"
	"fmt"
	"time"
)

type Checkpoint struct {
	SequenceNumber           CheckpointSequenceNumber
	Digest                   CheckpointDigest
	PreviousDigest           *CheckpointDigest
	Epoch                    uint64
	Timestamp                time.Time
	NetworkTotalTransactions uint64
}

type checkpointResponse struct {
	Checkpoint *struct {
		SequenceNumber           uint64  `json:"sequenceNumber"`
		Digest                   string  `json:"digest"`
		PreviousCheckpointDigest *string `json:"previousCheckpointDigest"`
		Epoch                    *struct {
			EpochID uint64 `json:"epochId"`
		} `json:"epoch"`
		Timestamp                string `json:"timestamp"`
		NetworkTotalTransactions uint64 `json:"networkTotalTransactions"`
	} `json:"checkpoint"`
}

// CheckpointBySequenceNumber returns a Sui checkpoint by sequence number.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - sequenceNumber: checkpoint sequence number.
//
// Returns:
//   - SDK-owned checkpoint.
//   - Retrieval or validation error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) CheckpointBySequenceNumber(ctx context.Context, sequenceNumber CheckpointSequenceNumber) (Checkpoint, error) {
	if err := sequenceNumber.Validate(); err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint by sequence number: %w", err)
	}
	query := fmt.Sprintf("query { checkpoint(sequenceNumber: %d) { sequenceNumber digest previousCheckpointDigest epoch { epochId } timestamp networkTotalTransactions } }", sequenceNumber)
	checkpoint, err := c.getCheckpoint(ctx, query)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint by sequence number: %w", err)
	}
	if checkpoint.SequenceNumber != sequenceNumber {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint by sequence number: sequence_number=mismatch")
	}
	return checkpoint, nil
}

// CheckpointByDigest returns a Sui checkpoint by digest.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - digest: checkpoint digest.
//
// Returns:
//   - SDK-owned checkpoint.
//   - Retrieval or validation error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) CheckpointByDigest(ctx context.Context, digest CheckpointDigest) (Checkpoint, error) {
	if digest.IsZero() {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint by digest: digest=empty")
	}
	query := fmt.Sprintf("query { checkpoint(digest: %q) { sequenceNumber digest previousCheckpointDigest epoch { epochId } timestamp networkTotalTransactions } }", digest.String())
	checkpoint, err := c.getCheckpoint(ctx, query)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint by digest: %w", err)
	}
	if checkpoint.Digest != digest {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint by digest: digest=mismatch")
	}
	return checkpoint, nil
}

func (c *RPCClient) getCheckpoint(ctx context.Context, query string) (Checkpoint, error) {
	if c == nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint: rpc_client=null")
	}
	if c.caller == nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint: checkpoint_provider=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var response checkpointResponse
	if err := c.caller.query(ctx, query, &response); err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint: %w", err)
	}
	if response.Checkpoint == nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint: checkpoint=null")
	}
	value := response.Checkpoint
	sequenceNumber := CheckpointSequenceNumber(value.SequenceNumber)
	if err := sequenceNumber.Validate(); err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint: %w", err)
	}
	digest, err := ParseCheckpointDigest(value.Digest)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint: %w", err)
	}
	if value.Epoch == nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint: epoch=null")
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value.Timestamp)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint: timestamp=invalid: %w", err)
	}
	var previousDigest *CheckpointDigest
	if value.PreviousCheckpointDigest != nil {
		parsed, err := ParseCheckpointDigest(*value.PreviousCheckpointDigest)
		if err != nil {
			return Checkpoint{}, fmt.Errorf("failed to get sui checkpoint: previous_digest=invalid: %w", err)
		}
		previousDigest = &parsed
	}
	return Checkpoint{
		SequenceNumber:           sequenceNumber,
		Digest:                   digest,
		PreviousDigest:           previousDigest,
		Epoch:                    value.Epoch.EpochID,
		Timestamp:                timestamp,
		NetworkTotalTransactions: value.NetworkTotalTransactions,
	}, nil
}
