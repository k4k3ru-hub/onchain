package sui

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"
)

type GasCostSummary struct {
	ComputationCost         *big.Int
	StorageCost             *big.Int
	StorageRebate           *big.Int
	NonRefundableStorageFee *big.Int
}

type BalanceChange struct {
	Address  Address
	CoinType string
	Amount   *big.Int
}

type TransactionEffects struct {
	Digest         TransactionDigest
	Successful     bool
	Error          *string
	Checkpoint     *CheckpointSequenceNumber
	Timestamp      *time.Time
	GasCost        GasCostSummary
	BalanceChanges []BalanceChange
}

// TransactionEffects returns the execution effects of a Sui transaction block.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - digest: transaction digest.
//
// Returns:
//   - SDK-owned transaction effects.
//   - Retrieval or validation error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) TransactionEffects(ctx context.Context, digest TransactionDigest) (*TransactionEffects, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to get sui transaction effects: rpc_client=null")
	}
	if c.caller == nil {
		return nil, fmt.Errorf("failed to get sui transaction effects: transaction_effects_provider=null")
	}
	if digest.IsZero() {
		return nil, fmt.Errorf("failed to get sui transaction effects: digest=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result struct {
		Transaction *struct {
			Digest  string `json:"digest"`
			Effects *struct {
				Status         string `json:"status"`
				ExecutionError *struct {
					Message string `json:"message"`
				} `json:"executionError"`
				Timestamp  *string `json:"timestamp"`
				Checkpoint *struct {
					SequenceNumber uint64 `json:"sequenceNumber"`
				} `json:"checkpoint"`
				GasEffects *struct {
					GasSummary *struct {
						ComputationCost         string `json:"computationCost"`
						StorageCost             string `json:"storageCost"`
						StorageRebate           string `json:"storageRebate"`
						NonRefundableStorageFee string `json:"nonRefundableStorageFee"`
					} `json:"gasSummary"`
				} `json:"gasEffects"`
				BalanceChanges struct {
					Nodes []struct {
						Owner *struct {
							Address string `json:"address"`
						} `json:"owner"`
						CoinType *struct {
							Representation string `json:"repr"`
						} `json:"coinType"`
						Amount string `json:"amount"`
					} `json:"nodes"`
				} `json:"balanceChanges"`
			} `json:"effects"`
		} `json:"transaction"`
	}
	query := fmt.Sprintf(`query { transaction(digest: %q) { digest effects { status executionError { message } timestamp checkpoint { sequenceNumber } gasEffects { gasSummary { computationCost storageCost storageRebate nonRefundableStorageFee } } balanceChanges { nodes { owner { address } coinType { repr } amount } } } } }`, digest.String())
	if err := c.caller.query(ctx, query, &result); err != nil {
		return nil, fmt.Errorf("failed to get sui transaction effects: %w", err)
	}
	if result.Transaction == nil {
		return nil, fmt.Errorf("failed to get sui transaction effects: transaction=null")
	}
	if result.Transaction.Digest != digest.String() {
		return nil, fmt.Errorf("failed to get sui transaction effects: digest=mismatch")
	}
	if result.Transaction.Effects == nil {
		return nil, fmt.Errorf("failed to get sui transaction effects: effects=null")
	}
	effects := result.Transaction.Effects
	status := strings.ToLower(effects.Status)
	if status != "success" && status != "failure" {
		return nil, fmt.Errorf("failed to get sui transaction effects: execution_status=invalid")
	}
	gasCost, err := parseGasCostSummary(effects.GasEffects)
	if err != nil {
		return nil, fmt.Errorf("failed to get sui transaction effects: %w", err)
	}
	output := &TransactionEffects{Digest: digest, Successful: status == "success", GasCost: gasCost}
	if effects.ExecutionError != nil {
		output.Error = &effects.ExecutionError.Message
	}
	if effects.Checkpoint != nil {
		checkpoint := CheckpointSequenceNumber(effects.Checkpoint.SequenceNumber)
		if err := checkpoint.Validate(); err != nil {
			return nil, fmt.Errorf("failed to get sui transaction effects: %w", err)
		}
		output.Checkpoint = &checkpoint
	}
	if effects.Timestamp != nil {
		parsed, err := time.Parse(time.RFC3339Nano, *effects.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to get sui transaction effects: timestamp=invalid: %w", err)
		}
		output.Timestamp = &parsed
	}
	for _, change := range effects.BalanceChanges.Nodes {
		if change.Owner == nil || change.CoinType == nil {
			return nil, fmt.Errorf("failed to get sui transaction effects: balance_change=invalid")
		}
		address, err := ParseAddress(change.Owner.Address)
		if err != nil {
			return nil, fmt.Errorf("failed to get sui transaction effects: %w", err)
		}
		amount, ok := new(big.Int).SetString(change.Amount, 10)
		if !ok || amount.Sign() == 0 || strings.TrimSpace(change.CoinType.Representation) == "" {
			return nil, fmt.Errorf("failed to get sui transaction effects: balance_change=invalid")
		}
		output.BalanceChanges = append(output.BalanceChanges, BalanceChange{Address: address, CoinType: change.CoinType.Representation, Amount: amount})
	}
	return output, nil
}

func parseGasCostSummary(value *struct {
	GasSummary *struct {
		ComputationCost         string `json:"computationCost"`
		StorageCost             string `json:"storageCost"`
		StorageRebate           string `json:"storageRebate"`
		NonRefundableStorageFee string `json:"nonRefundableStorageFee"`
	} `json:"gasSummary"`
}) (GasCostSummary, error) {
	if value == nil || value.GasSummary == nil {
		return GasCostSummary{}, fmt.Errorf("failed to parse sui gas cost summary: gas_summary=null")
	}
	parse := func(field, raw string) (*big.Int, error) {
		parsed, ok := new(big.Int).SetString(raw, 10)
		if !ok || parsed.Sign() < 0 {
			return nil, fmt.Errorf("failed to parse sui gas cost summary: %s=invalid", field)
		}
		return parsed, nil
	}
	computationCost, err := parse("computation_cost", value.GasSummary.ComputationCost)
	if err != nil {
		return GasCostSummary{}, err
	}
	storageCost, err := parse("storage_cost", value.GasSummary.StorageCost)
	if err != nil {
		return GasCostSummary{}, err
	}
	storageRebate, err := parse("storage_rebate", value.GasSummary.StorageRebate)
	if err != nil {
		return GasCostSummary{}, err
	}
	nonRefundableStorageFee, err := parse("non_refundable_storage_fee", value.GasSummary.NonRefundableStorageFee)
	if err != nil {
		return GasCostSummary{}, err
	}
	return GasCostSummary{ComputationCost: computationCost, StorageCost: storageCost, StorageRebate: storageRebate, NonRefundableStorageFee: nonRefundableStorageFee}, nil
}

// IsSuccessful reports whether the transaction executed successfully.
func (e *TransactionEffects) IsSuccessful() bool { return e != nil && e.Successful }

// IsFinalized reports whether the transaction is included in a checkpoint.
func (e *TransactionEffects) IsFinalized() bool { return e != nil && e.Checkpoint != nil }
