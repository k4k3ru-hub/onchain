package sui

import (
	"context"
	"fmt"
	"strings"
)

type TransactionStatus struct {
	Digest     TransactionDigest
	Checkpoint *CheckpointSequenceNumber
	Successful bool
	Error      *string
}

// TransactionStatus returns the execution status of a Sui transaction block.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - digest: transaction digest.
//
// Returns:
//   - Transaction execution status.
//   - Retrieval error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) TransactionStatus(ctx context.Context, digest TransactionDigest) (*TransactionStatus, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to get sui transaction status: rpc_client=null")
	}
	if c.caller == nil {
		return nil, fmt.Errorf("failed to get sui transaction status: transaction_status_provider=null")
	}
	if digest.IsZero() {
		return nil, fmt.Errorf("failed to get sui transaction status: digest=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result struct {
		Transaction *struct {
			Digest  string `json:"digest"`
			Effects *struct {
				Status     string `json:"status"`
				Checkpoint *struct {
					SequenceNumber uint64 `json:"sequenceNumber"`
				} `json:"checkpoint"`
			} `json:"effects"`
		} `json:"transaction"`
	}
	query := fmt.Sprintf(`query { transaction(digest: %q) { digest effects { status checkpoint { sequenceNumber } } } }`, digest.String())
	if err := c.caller.query(ctx, query, &result); err != nil {
		return nil, fmt.Errorf("failed to get sui transaction status: %w", err)
	}
	if result.Transaction == nil {
		return nil, fmt.Errorf("failed to get sui transaction status: transaction=null")
	}
	if result.Transaction.Digest != digest.String() {
		return nil, fmt.Errorf("failed to get sui transaction status: digest=mismatch")
	}
	if result.Transaction.Effects == nil {
		return nil, fmt.Errorf("failed to get sui transaction status: effects=null")
	}
	executionStatus := strings.ToLower(result.Transaction.Effects.Status)
	if executionStatus != "success" && executionStatus != "failure" {
		return nil, fmt.Errorf("failed to get sui transaction status: execution_status=invalid")
	}
	var checkpoint *CheckpointSequenceNumber
	if result.Transaction.Effects.Checkpoint != nil {
		parsed := CheckpointSequenceNumber(result.Transaction.Effects.Checkpoint.SequenceNumber)
		checkpoint = &parsed
	}
	return &TransactionStatus{
		Digest: digest, Checkpoint: checkpoint,
		Successful: executionStatus == "success",
	}, nil
}

// IsSuccessful reports whether the Sui transaction executed successfully.
func (s *TransactionStatus) IsSuccessful() bool { return s != nil && s.Successful }

// IsFinalized reports whether the Sui transaction is included in a checkpoint.
func (s *TransactionStatus) IsFinalized() bool { return s != nil && s.Checkpoint != nil }
