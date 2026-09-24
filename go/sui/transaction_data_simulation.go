package sui

import (
	"bytes"
	"context"
	"fmt"
	"math/big"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type TransactionDataSimulationResult struct {
	TransactionBytes []byte
	GasCost          GasCostSummary
	InputObjects     []SimulationObjectReference
	BalanceChanges   []BalanceChange
	Events           []SimulationEvent
}

type transactionDataSimulationProvider interface {
	simulateTransactionData(context.Context, []byte) (*TransactionDataSimulationResult, error)
}

// SimulateTransactionData checks a complete transaction using exactly its selected
// objects and gas budget. It enables execution checks and disables gas selection.
// A successful simulation is not a submission, signature check, or checkpoint lock.
//
// Version:
//   - 2026-09-24: Added.
func (c *GRPCClient) SimulateTransactionData(ctx context.Context, transaction TransactionData) (*TransactionDataSimulationResult, error) {
	if c == nil || c.transactionDataSimulationProvider == nil {
		return nil, fmt.Errorf("failed to simulate sui transaction data: provider=null")
	}
	data, err := transaction.MarshalBCS()
	if err != nil {
		return nil, fmt.Errorf("failed to simulate sui transaction data: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := c.transactionDataSimulationProvider.simulateTransactionData(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("failed to simulate sui transaction data: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("failed to simulate sui transaction data: result=null")
	}
	return result, nil
}

func (a *grpcAdapter) simulateTransactionData(ctx context.Context, data []byte) (*TransactionDataSimulationResult, error) {
	if a == nil || a.executionClient == nil {
		return nil, fmt.Errorf("failed to call sui transaction data simulation: execution_client=null")
	}
	selectGas := false
	response, err := a.executionClient.SimulateTransaction(ctx, &rpcv2.SimulateTransactionRequest{
		Transaction: &rpcv2.Transaction{Bcs: &rpcv2.Bcs{Value: append([]byte(nil), data...)}},
		Checks:      rpcv2.SimulateTransactionRequest_ENABLED.Enum(), DoGasSelection: &selectGas,
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"transaction.transaction.bcs", "transaction.effects", "transaction.events", "transaction.balance_changes"}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call sui transaction data simulation: %w", err)
	}
	if response == nil || response.Transaction == nil || response.Transaction.Effects == nil || response.Transaction.Effects.Status == nil || response.Transaction.Effects.Status.Success == nil {
		return nil, fmt.Errorf("failed to call sui transaction data simulation: effects=null")
	}
	tx := response.Transaction
	if !tx.Effects.Status.GetSuccess() {
		return nil, fmt.Errorf("failed to call sui transaction data simulation: %w", simulationExecutionError(tx.Effects.Status.GetError()))
	}
	if tx.Transaction == nil || tx.Transaction.Bcs == nil || !bytes.Equal(tx.Transaction.Bcs.Value, data) {
		return nil, fmt.Errorf("failed to call sui transaction data simulation: transaction_bytes=mismatch")
	}
	gas := tx.Effects.GasUsed
	if gas == nil || gas.ComputationCost == nil || gas.StorageCost == nil || gas.StorageRebate == nil || gas.NonRefundableStorageFee == nil {
		return nil, fmt.Errorf("failed to call sui transaction data simulation: gas_cost=null")
	}
	inputs, err := simulationInputObjects(tx.Effects)
	if err != nil {
		return nil, fmt.Errorf("failed to call sui transaction data simulation: %w", err)
	}
	result := &TransactionDataSimulationResult{TransactionBytes: append([]byte(nil), data...), InputObjects: inputs, GasCost: GasCostSummary{
		ComputationCost: new(big.Int).SetUint64(gas.GetComputationCost()), StorageCost: new(big.Int).SetUint64(gas.GetStorageCost()),
		StorageRebate: new(big.Int).SetUint64(gas.GetStorageRebate()), NonRefundableStorageFee: new(big.Int).SetUint64(gas.GetNonRefundableStorageFee()),
	}}
	for _, change := range tx.BalanceChanges {
		if change == nil {
			return nil, fmt.Errorf("failed to call sui transaction data simulation: balance_change=null")
		}
		owner, err := ParseAddress(change.GetAddress())
		if err != nil {
			return nil, fmt.Errorf("failed to call sui transaction data simulation: %w", err)
		}
		tag, err := parseTransactionTypeTag(change.GetCoinType())
		if err != nil {
			return nil, fmt.Errorf("failed to call sui transaction data simulation: %w", err)
		}
		amount, ok := new(big.Int).SetString(change.GetAmount(), 10)
		if !ok {
			return nil, fmt.Errorf("failed to call sui transaction data simulation: balance_amount=invalid")
		}
		result.BalanceChanges = append(result.BalanceChanges, BalanceChange{Address: owner, CoinType: tag.text(), Amount: amount})
	}
	if tx.Events != nil {
		for _, event := range tx.Events.Events {
			if event == nil {
				return nil, fmt.Errorf("failed to call sui transaction data simulation: event=null")
			}
			address, err := ParseAddress(event.GetPackageId())
			if err != nil {
				return nil, fmt.Errorf("failed to call sui transaction data simulation: %w", err)
			}
			value := SimulationEvent{Package: address, Module: event.GetModule(), Type: event.GetEventType()}
			if event.Contents != nil {
				value.BCS = append([]byte(nil), event.Contents.Value...)
			}
			if event.Json != nil {
				value.JSON = event.Json.AsInterface()
			}
			result.Events = append(result.Events, value)
		}
	}
	return result, nil
}
