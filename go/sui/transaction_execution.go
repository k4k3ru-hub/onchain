package sui

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// TransactionExecutionResult reports execution effects, not an AMM fill.
type TransactionExecutionResult struct {
	Digest  TransactionDigest
	Success bool
}

type transactionExecutionProvider interface {
	executeTransaction(context.Context, []byte, []byte) (*TransactionExecutionResult, error)
}

// ExecuteTransaction verifies and sends an Ed25519, sender-paid transaction.
// Retry uncertain transport failures with the identical bytes and signature.
// A nil error means effects were returned; inspect Success for execution failure.
//
// Version:
//   - 2026-09-25: Added.
func (c *GRPCClient) ExecuteTransaction(ctx context.Context, data, signature []byte) (*TransactionExecutionResult, error) {
	if c == nil || c.transactionExecutionProvider == nil {
		return nil, fmt.Errorf("failed to execute sui transaction: provider=null")
	}
	tx, err := ParseTransactionData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sui transaction: %w", err)
	}
	if tx.Sender != tx.GasData.Owner {
		return nil, fmt.Errorf("failed to execute sui transaction: sponsored transactions are not supported")
	}
	if err := VerifyTransactionSignature(data, signature, tx.Sender); err != nil {
		return nil, fmt.Errorf("failed to execute sui transaction: %w", err)
	}
	digest, err := tx.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to execute sui transaction: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := c.transactionExecutionProvider.executeTransaction(ctx, data, signature)
	if err != nil {
		return nil, fmt.Errorf("failed to execute sui transaction: %w", err)
	}
	if result == nil || result.Digest != digest {
		return nil, fmt.Errorf("failed to execute sui transaction: transaction_digest=mismatch")
	}
	return result, nil
}

func (a *grpcAdapter) executeTransaction(ctx context.Context, data, signature []byte) (*TransactionExecutionResult, error) {
	if a == nil || a.executionClient == nil {
		return nil, fmt.Errorf("failed to call sui transaction execution: execution_client=null")
	}
	response, err := a.executionClient.ExecuteTransaction(ctx, &rpcv2.ExecuteTransactionRequest{
		Transaction: &rpcv2.Transaction{Bcs: &rpcv2.Bcs{Value: append([]byte(nil), data...)}},
		Signatures:  []*rpcv2.UserSignature{{Bcs: &rpcv2.Bcs{Value: append([]byte(nil), signature...)}}},
		ReadMask:    &fieldmaskpb.FieldMask{Paths: []string{"digest", "effects.status"}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call sui transaction execution: %w", err)
	}
	if response == nil || response.Transaction == nil || response.Transaction.Effects == nil || response.Transaction.Effects.Status == nil || response.Transaction.Effects.Status.Success == nil {
		return nil, fmt.Errorf("failed to call sui transaction execution: effects=null")
	}
	digest, err := ParseTransactionDigest(response.Transaction.GetDigest())
	if err != nil {
		return nil, fmt.Errorf("failed to call sui transaction execution: %w", err)
	}
	return &TransactionExecutionResult{Digest: digest, Success: response.Transaction.Effects.Status.GetSuccess()}, nil
}
