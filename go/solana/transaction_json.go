package solana

import (
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/gagliardetto/solana-go"
	rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
)

var ErrUnsupportedTransactionVersion = errors.New("failed to decode solana transaction: version=unsupported")

// Only fields consumed by Transaction are decoded. v1 transactionConfig and
// instructions are not needed for account identities, logs or token balances.
type jsonTransactionResult struct {
	Slot        uint64               `json:"slot"`
	BlockTime   *int64               `json:"blockTime"`
	Version     json.RawMessage      `json:"version"`
	Meta        *rpc.TransactionMeta `json:"meta"`
	Transaction *struct {
		Signatures []sdk.Signature `json:"signatures"`
		Message    struct {
			AccountKeys sdk.PublicKeySlice `json:"accountKeys"`
		} `json:"message"`
	} `json:"transaction"`
}

func (r *jsonTransactionResult) validate(signature Signature) error {
	version := -1
	if len(r.Version) > 0 && string(r.Version) != `"legacy"` {
		if err := json.Unmarshal(r.Version, &version); err != nil {
			return fmt.Errorf("failed to decode solana transaction version: %w", err)
		}
		if version < 0 || version > 1 {
			return ErrUnsupportedTransactionVersion
		}
	}
	if len(r.Transaction.Signatures) == 0 || r.Transaction.Signatures[0].String() != signature.String() {
		return fmt.Errorf("failed to decode solana transaction: signature=invalid")
	}
	if len(r.Transaction.Message.AccountKeys) == 0 || r.Meta == nil {
		return fmt.Errorf("failed to decode solana transaction: metadata=invalid")
	}
	if version == 1 && (len(r.Meta.LoadedAddresses.Writable) > 0 || len(r.Meta.LoadedAddresses.ReadOnly) > 0) {
		return fmt.Errorf("failed to decode solana transaction: loaded_addresses=invalid version=1")
	}
	count := len(r.Transaction.Message.AccountKeys) + len(r.Meta.LoadedAddresses.Writable) + len(r.Meta.LoadedAddresses.ReadOnly)
	for _, balances := range [][]rpc.TokenBalance{r.Meta.PreTokenBalances, r.Meta.PostTokenBalances} {
		for _, balance := range balances {
			if int(balance.AccountIndex) >= count {
				return fmt.Errorf("failed to decode solana transaction: account_index=out_of_range")
			}
		}
	}
	return nil
}

// IsPermanentTransactionError reports request failures that cannot improve on retry.
// Wrapped provider errors remain inspectable; transport and availability errors
// are not classified as permanent.
//
// Version:
//   - 2026-09-17: Added.
func IsPermanentTransactionError(err error) bool {
	if errors.Is(err, ErrUnsupportedTransactionVersion) {
		return true
	}
	var provider *jsonrpc.RPCError
	if !errors.As(err, &provider) {
		return false
	}
	switch provider.Code {
	case -32015, -32600, -32601, -32602:
		return true
	}
	return false
}
