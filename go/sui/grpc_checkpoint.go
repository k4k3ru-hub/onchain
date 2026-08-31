package sui

import (
	"context"
	"fmt"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type grpcCheckpointProvider interface {
	checkpointBySequenceNumber(context.Context, CheckpointSequenceNumber) (Checkpoint, error)
}

// CheckpointBySequenceNumber returns a Sui checkpoint through gRPC.
//
// Parameters:
//   - ctx: Request context; nil uses context.Background.
//   - sequenceNumber: Checkpoint sequence number.
//
// Returns:
//   - SDK-owned checkpoint.
//   - Retrieval or validation error.
//
// Version:
//   - 2026-08-31: Added.
func (c *GRPCClient) CheckpointBySequenceNumber(ctx context.Context, sequenceNumber CheckpointSequenceNumber) (Checkpoint, error) {
	if c == nil || c.checkpointProvider == nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui grpc checkpoint by sequence number: checkpoint_provider=null")
	}
	if err := sequenceNumber.Validate(); err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui grpc checkpoint by sequence number: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	checkpoint, err := c.checkpointProvider.checkpointBySequenceNumber(ctx, sequenceNumber)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("failed to get sui grpc checkpoint by sequence number: %w", err)
	}
	return checkpoint, nil
}

func (a *grpcAdapter) checkpointBySequenceNumber(ctx context.Context, sequenceNumber CheckpointSequenceNumber) (Checkpoint, error) {
	if a == nil || a.ledgerClient == nil {
		return Checkpoint{}, fmt.Errorf("failed to call sui grpc checkpoint: ledger_client=null")
	}
	response, err := a.ledgerClient.GetCheckpoint(ctx, &rpcv2.GetCheckpointRequest{
		CheckpointId: &rpcv2.GetCheckpointRequest_SequenceNumber{SequenceNumber: sequenceNumber.Uint64()},
		ReadMask:     &fieldmaskpb.FieldMask{Paths: []string{"sequence_number", "digest", "summary.epoch", "summary.timestamp", "summary.previous_digest", "summary.total_network_transactions"}},
	})
	if err != nil {
		return Checkpoint{}, fmt.Errorf("failed to call sui grpc checkpoint: %w", err)
	}
	if response == nil || response.Checkpoint == nil || response.Checkpoint.Summary == nil {
		return Checkpoint{}, fmt.Errorf("failed to call sui grpc checkpoint: checkpoint=null")
	}
	value := response.Checkpoint
	if value.GetSequenceNumber() != sequenceNumber.Uint64() {
		return Checkpoint{}, fmt.Errorf("failed to call sui grpc checkpoint: sequence_number=mismatch")
	}
	digest, err := ParseCheckpointDigest(value.GetDigest())
	if err != nil {
		return Checkpoint{}, fmt.Errorf("failed to call sui grpc checkpoint: %w", err)
	}
	summary := value.GetSummary()
	if summary.GetTimestamp() == nil {
		return Checkpoint{}, fmt.Errorf("failed to call sui grpc checkpoint: timestamp=null")
	}
	checkpoint := Checkpoint{SequenceNumber: sequenceNumber, Digest: digest, Epoch: summary.GetEpoch(), Timestamp: time.Unix(summary.GetTimestamp().Seconds, int64(summary.GetTimestamp().Nanos)).UTC(), NetworkTotalTransactions: summary.GetTotalNetworkTransactions()}
	if previous := summary.GetPreviousDigest(); previous != "" {
		parsed, parseErr := ParseCheckpointDigest(previous)
		if parseErr != nil {
			return Checkpoint{}, fmt.Errorf("failed to call sui grpc checkpoint: %w", parseErr)
		}
		checkpoint.PreviousDigest = &parsed
	}
	return checkpoint, nil
}
