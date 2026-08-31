package sui

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
)

type grpcCheckpointLedgerStub struct {
	rpcv2.LedgerServiceClient
	response *rpcv2.GetCheckpointResponse
}

func (s grpcCheckpointLedgerStub) GetCheckpoint(context.Context, *rpcv2.GetCheckpointRequest, ...grpc.CallOption) (*rpcv2.GetCheckpointResponse, error) {
	return s.response, nil
}

func TestGRPCCheckpointBySequenceNumber(t *testing.T) {
	digestBytes := CheckpointDigest{1}
	previousBytes := CheckpointDigest{2}
	sequence := uint64(123)
	digest := digestBytes.String()
	previous := previousBytes.String()
	epoch := uint64(9)
	total := uint64(456)
	client := composeGRPCClient(GRPCConfig{URL: "https://example.com"}, &grpcAdapter{ledgerClient: grpcCheckpointLedgerStub{response: &rpcv2.GetCheckpointResponse{Checkpoint: &rpcv2.Checkpoint{
		SequenceNumber: &sequence,
		Digest:         &digest,
		Summary:        &rpcv2.CheckpointSummary{Epoch: &epoch, PreviousDigest: &previous, TotalNetworkTransactions: &total, Timestamp: &timestamp.Timestamp{Seconds: 789}},
	}}}}, nil)
	checkpoint, err := client.CheckpointBySequenceNumber(context.Background(), CheckpointSequenceNumber(sequence))
	if err != nil {
		t.Fatalf("CheckpointBySequenceNumber() returned an unexpected error: %v", err)
	}
	if checkpoint.SequenceNumber.Uint64() != sequence || checkpoint.Digest != digestBytes || checkpoint.PreviousDigest == nil || *checkpoint.PreviousDigest != previousBytes || checkpoint.Epoch != epoch || checkpoint.Timestamp.Unix() != 789 || checkpoint.NetworkTotalTransactions != total {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
}
