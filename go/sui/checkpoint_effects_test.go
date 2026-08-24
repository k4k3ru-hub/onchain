package sui

import (
	"context"
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

func TestCheckpointBySequenceNumber(t *testing.T) {
	digestValue := base58.Encode(bytesWithFirstByte(1))
	previousValue := base58.Encode(bytesWithFirstByte(2))
	caller := &fakeCaller{response: map[string]any{"checkpoint": map[string]any{
		"sequenceNumber": 42, "digest": digestValue, "previousCheckpointDigest": previousValue,
		"epoch": map[string]any{"epochId": 7}, "timestamp": "2026-08-22T12:34:56.123Z",
		"networkTotalTransactions": 1234,
	}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	checkpoint, err := client.CheckpointBySequenceNumber(nil, 42)
	if err != nil {
		t.Fatalf("CheckpointBySequenceNumber() returned an unexpected error: %v", err)
	}
	if checkpoint.SequenceNumber != 42 || checkpoint.Digest.String() != digestValue || checkpoint.PreviousDigest == nil || checkpoint.PreviousDigest.String() != previousValue {
		t.Fatalf("CheckpointBySequenceNumber() = %+v, want matching checkpoint", checkpoint)
	}
	if checkpoint.Epoch != 7 || checkpoint.NetworkTotalTransactions != 1234 || checkpoint.Timestamp.IsZero() {
		t.Fatalf("CheckpointBySequenceNumber() metadata = %+v", checkpoint)
	}
	if !strings.Contains(caller.queryValue, "checkpoint(sequenceNumber: 42)") {
		t.Fatalf("CheckpointBySequenceNumber() query = %q", caller.queryValue)
	}
}

func TestCheckpointByDigest(t *testing.T) {
	digestValue := base58.Encode(bytesWithFirstByte(3))
	digest, err := ParseCheckpointDigest(digestValue)
	if err != nil {
		t.Fatalf("ParseCheckpointDigest() returned an unexpected error: %v", err)
	}
	caller := &fakeCaller{response: map[string]any{"checkpoint": map[string]any{
		"sequenceNumber": 9, "digest": digestValue, "epoch": map[string]any{"epochId": 1},
		"timestamp": "2026-08-22T12:34:56Z", "networkTotalTransactions": 10,
	}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	checkpoint, err := client.CheckpointByDigest(context.Background(), digest)
	if err != nil {
		t.Fatalf("CheckpointByDigest() returned an unexpected error: %v", err)
	}
	if checkpoint.Digest != digest || !strings.Contains(caller.queryValue, digest.String()) {
		t.Fatalf("CheckpointByDigest() = %+v query=%q", checkpoint, caller.queryValue)
	}
}

func TestTransactionEffects(t *testing.T) {
	var digest TransactionDigest
	digest[0] = 4
	errorMessage := "MoveAbort"
	owner, _ := ParseAddress("0x123")
	caller := &fakeCaller{response: map[string]any{"transaction": map[string]any{
		"digest": digest.String(),
		"effects": map[string]any{
			"status": "FAILURE", "executionError": map[string]any{"message": errorMessage}, "timestamp": "2026-08-22T12:34:56Z",
			"checkpoint": map[string]any{"sequenceNumber": 99},
			"gasEffects": map[string]any{"gasSummary": map[string]any{
				"computationCost": "100", "storageCost": "20", "storageRebate": "5", "nonRefundableStorageFee": "1",
			}},
			"balanceChanges": map[string]any{"nodes": []any{map[string]any{
				"owner": map[string]any{"address": owner.String()}, "coinType": map[string]any{"repr": "0x2::sui::SUI"}, "amount": "-100",
			}}},
		},
	}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	effects, err := client.TransactionEffects(nil, digest)
	if err != nil {
		t.Fatalf("TransactionEffects() returned an unexpected error: %v", err)
	}
	if effects.IsSuccessful() || !effects.IsFinalized() || effects.Error == nil || *effects.Error != errorMessage {
		t.Fatalf("TransactionEffects() = %+v, want finalized failure", effects)
	}
	if effects.GasCost.ComputationCost.String() != "100" || effects.GasCost.StorageRebate.String() != "5" {
		t.Fatalf("TransactionEffects() gas cost = %+v", effects.GasCost)
	}
	if len(effects.BalanceChanges) != 1 || effects.BalanceChanges[0].Address != owner || effects.BalanceChanges[0].Amount.String() != "-100" {
		t.Fatalf("TransactionEffects() balance changes = %+v", effects.BalanceChanges)
	}
}

func TestTransactionEffectsRejectsInvalidGasCost(t *testing.T) {
	var digest TransactionDigest
	digest[0] = 5
	caller := &fakeCaller{response: map[string]any{"transaction": map[string]any{
		"digest": digest.String(), "effects": map[string]any{
			"status": "SUCCESS", "gasEffects": map[string]any{"gasSummary": map[string]any{
				"computationCost": "invalid", "storageCost": "0", "storageRebate": "0", "nonRefundableStorageFee": "0",
			}},
		},
	}}}
	client := composeRPCClient(RPCConfig{URL: "https://example.com/graphql"}, caller)
	if _, err := client.TransactionEffects(nil, digest); err == nil || !strings.Contains(err.Error(), "computation_cost=invalid") {
		t.Fatalf("TransactionEffects() error = %v, want invalid computation cost", err)
	}
}

func bytesWithFirstByte(value byte) []byte {
	result := make([]byte, digestByteLength)
	result[0] = value
	return result
}
