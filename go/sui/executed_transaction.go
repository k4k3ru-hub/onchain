package sui

import (
	"context"
	"fmt"
	"math/big"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ExecutedTransaction contains ledger evidence, without interpreting a venue fill.
// Checkpoint and Timestamp remain nil until checkpoint inclusion is reported.
type ExecutedTransaction struct {
	TransactionBytes []byte
	Effects          TransactionEffects
	Events           []TransactionEvent
}

// TransactionEvent retains event order and BCS without converting integer fields to floats.
type TransactionEvent struct {
	Package Address
	Module  string
	Sender  Address
	Type    string
	BCS     []byte
}

type executedTransactionProvider interface {
	executedTransaction(context.Context, TransactionDigest) (*ExecutedTransaction, error)
}

// ExecutedTransaction reads transaction bytes, effects, balance changes, and BCS events.
// A nil result with a nil error means this node did not find the transaction; it
// does not prove that submission failed. Transport errors remain inspectable.
// The returned bytes are checked against the requested digest, without limiting
// reads to the programmable transaction commands supported by ParseTransactionData.
//
// Version:
//   - 2026-09-25: Added.
func (c *GRPCClient) ExecutedTransaction(ctx context.Context, digest TransactionDigest) (*ExecutedTransaction, error) {
	if c == nil || c.executedTransactionProvider == nil {
		return nil, fmt.Errorf("failed to read sui executed transaction: provider=null")
	}
	if digest.IsZero() {
		return nil, fmt.Errorf("failed to read sui executed transaction: digest=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := c.executedTransactionProvider.executedTransaction(ctx, digest)
	if err != nil {
		return nil, fmt.Errorf("failed to read sui executed transaction: %w", err)
	}
	if result == nil {
		return nil, nil
	}
	if len(result.TransactionBytes) == 0 || result.Effects.Digest != digest || TransactionDigest(blake2b.Sum256(append([]byte("TransactionData::"), result.TransactionBytes...))) != digest {
		return nil, fmt.Errorf("failed to read sui executed transaction: transaction_digest=mismatch")
	}
	return result, nil
}

func (a *grpcAdapter) executedTransaction(ctx context.Context, digest TransactionDigest) (*ExecutedTransaction, error) {
	if a == nil || a.ledgerClient == nil {
		return nil, fmt.Errorf("failed to call sui executed transaction: ledger_client=null")
	}
	value := digest.String()
	response, err := a.ledgerClient.GetTransaction(ctx, &rpcv2.GetTransactionRequest{
		Digest: &value,
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{
			"digest", "transaction.bcs", "effects.status", "effects.transaction_digest", "effects.gas_used", "effects.events_digest",
			"checkpoint", "timestamp", "transaction_index", "balance_changes", "events",
		}},
	})
	if status.Code(err) == codes.NotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to call sui executed transaction: %w", err)
	}
	if response == nil || response.Transaction == nil {
		return nil, fmt.Errorf("failed to decode sui executed transaction: transaction=null")
	}
	tx := response.Transaction
	if tx.Transaction == nil || tx.Transaction.Bcs == nil || len(tx.Transaction.Bcs.Value) == 0 {
		return nil, fmt.Errorf("failed to decode sui executed transaction: transaction_bytes=empty")
	}
	if tx.Effects == nil || tx.Effects.Status == nil || tx.Effects.Status.Success == nil {
		return nil, fmt.Errorf("failed to decode sui executed transaction: effects=null")
	}
	if tx.GetDigest() != digest.String() || tx.Effects.GetTransactionDigest() != digest.String() {
		return nil, fmt.Errorf("failed to decode sui executed transaction: transaction_digest=mismatch")
	}
	gas := tx.Effects.GasUsed
	if gas == nil || gas.ComputationCost == nil || gas.StorageCost == nil || gas.StorageRebate == nil || gas.NonRefundableStorageFee == nil {
		return nil, fmt.Errorf("failed to decode sui executed transaction: gas_cost=null")
	}
	result := &ExecutedTransaction{
		TransactionBytes: append([]byte(nil), tx.Transaction.Bcs.Value...),
		Effects: TransactionEffects{Digest: digest, Successful: tx.Effects.Status.GetSuccess(), GasCost: GasCostSummary{
			ComputationCost: new(big.Int).SetUint64(gas.GetComputationCost()), StorageCost: new(big.Int).SetUint64(gas.GetStorageCost()),
			StorageRebate: new(big.Int).SetUint64(gas.GetStorageRebate()), NonRefundableStorageFee: new(big.Int).SetUint64(gas.GetNonRefundableStorageFee()),
		}},
	}
	if (tx.Checkpoint == nil) != (tx.Timestamp == nil) {
		return nil, fmt.Errorf("failed to decode sui executed transaction: checkpoint_timestamp=invalid")
	}
	if tx.Checkpoint != nil {
		if err := tx.Timestamp.CheckValid(); err != nil {
			return nil, fmt.Errorf("failed to decode sui executed transaction: %w", err)
		}
		checkpoint, timestamp := CheckpointSequenceNumber(*tx.Checkpoint), tx.Timestamp.AsTime()
		result.Effects.Checkpoint, result.Effects.Timestamp = &checkpoint, &timestamp
	}
	if tx.TransactionIndex != nil {
		index := *tx.TransactionIndex
		result.Effects.TransactionIndex = &index
	}
	for _, change := range tx.BalanceChanges {
		if change == nil {
			return nil, fmt.Errorf("failed to decode sui executed transaction: balance_change=null")
		}
		owner, err := ParseAddress(change.GetAddress())
		if err != nil {
			return nil, fmt.Errorf("failed to decode sui executed transaction: %w", err)
		}
		coinType, err := NormalizeMoveType(change.GetCoinType())
		if err != nil {
			return nil, fmt.Errorf("failed to decode sui executed transaction: %w", err)
		}
		text := change.GetAmount()
		if len(text) == 0 || len(text) > 79 {
			return nil, fmt.Errorf("failed to decode sui executed transaction: balance_amount=invalid")
		}
		amount, ok := new(big.Int).SetString(text, 10)
		if !ok || amount.String() != text {
			return nil, fmt.Errorf("failed to decode sui executed transaction: balance_amount=invalid")
		}
		result.Effects.BalanceChanges = append(result.Effects.BalanceChanges, BalanceChange{Address: owner, CoinType: coinType, Amount: amount})
	}
	if tx.Effects.GetEventsDigest() != "" && (tx.Events == nil || len(tx.Events.Events) == 0) {
		return nil, fmt.Errorf("failed to decode sui executed transaction: events=empty")
	}
	if tx.Events != nil {
		for _, event := range tx.Events.Events {
			if event == nil || event.Contents == nil {
				return nil, fmt.Errorf("failed to decode sui executed transaction: event=null")
			}
			pkg, err := ParseAddress(event.GetPackageId())
			if err != nil {
				return nil, fmt.Errorf("failed to decode sui executed transaction: %w", err)
			}
			sender, err := ParseAddress(event.GetSender())
			if err != nil {
				return nil, fmt.Errorf("failed to decode sui executed transaction: %w", err)
			}
			eventType, err := NormalizeMoveType(event.GetEventType())
			if err != nil {
				return nil, fmt.Errorf("failed to decode sui executed transaction: %w", err)
			}
			if !validMoveIdentifier(event.GetModule()) {
				return nil, fmt.Errorf("failed to decode sui executed transaction: module=invalid")
			}
			result.Events = append(result.Events, TransactionEvent{Package: pkg, Module: event.GetModule(), Sender: sender, Type: eventType, BCS: append([]byte(nil), event.Contents.Value...)})
		}
	}
	return result, nil
}
