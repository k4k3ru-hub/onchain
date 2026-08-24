package sui

import (
	"context"
	"fmt"
	"strconv"
)

type CheckpointSequenceNumber uint64

// Uint64 returns the numeric checkpoint sequence number.
func (n CheckpointSequenceNumber) Uint64() uint64 { return uint64(n) }

// String returns the decimal checkpoint sequence number.
func (n CheckpointSequenceNumber) String() string { return strconv.FormatUint(n.Uint64(), 10) }

// Validate validates a Sui checkpoint sequence number.
//
// Version:
//   - 2026-08-22: Allowed the genesis checkpoint at sequence number zero.
func (n CheckpointSequenceNumber) Validate() error {
	return nil
}

// LatestCheckpointSequenceNumber returns the latest Sui checkpoint sequence number.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//
// Returns:
//   - Latest checkpoint sequence number.
//   - Retrieval error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) LatestCheckpointSequenceNumber(ctx context.Context) (CheckpointSequenceNumber, error) {
	if c == nil {
		return 0, fmt.Errorf("failed to get latest sui checkpoint sequence number: rpc_client=null")
	}
	if c.caller == nil {
		return 0, fmt.Errorf("failed to get latest sui checkpoint sequence number: checkpoint_provider=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result struct {
		Checkpoint *struct {
			SequenceNumber uint64 `json:"sequenceNumber"`
		} `json:"checkpoint"`
	}
	if err := c.caller.query(ctx, "query { checkpoint { sequenceNumber } }", &result); err != nil {
		return 0, fmt.Errorf("failed to get latest sui checkpoint sequence number: %w", err)
	}
	if result.Checkpoint == nil {
		return 0, fmt.Errorf("failed to get latest sui checkpoint sequence number: checkpoint=null")
	}
	sequenceNumber := CheckpointSequenceNumber(result.Checkpoint.SequenceNumber)
	if err := sequenceNumber.Validate(); err != nil {
		return 0, fmt.Errorf("failed to get latest sui checkpoint sequence number: %w", err)
	}
	return sequenceNumber, nil
}
