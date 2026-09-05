package evm

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type DynamicFeeTransactionParams struct {
	ChainID   ChainID
	Nonce     uint64
	GasTipCap *big.Int
	GasFeeCap *big.Int
	GasLimit  uint64
	To        common.Address
	Value     *big.Int
	Data      []byte
}

type UnsignedDynamicFeeTransaction struct {
	chainID *big.Int
	value   *types.Transaction
}

// NewUnsignedDynamicFeeTransaction builds an unsigned EIP-1559 transaction.
//
// Parameters:
//   - params: Dynamic-fee transaction fields.
//
// Returns:
//   - Immutable unsigned transaction wrapper.
//   - Validation error.
//
// Version:
//   - 2026-09-05: Added.
func NewUnsignedDynamicFeeTransaction(params DynamicFeeTransactionParams) (*UnsignedDynamicFeeTransaction, error) {
	if params.ChainID == 0 {
		return nil, fmt.Errorf("failed to create unsigned evm dynamic fee transaction: chain_id=empty")
	}
	if params.GasTipCap == nil {
		return nil, fmt.Errorf("failed to create unsigned evm dynamic fee transaction: gas_tip_cap=null")
	}
	if params.GasTipCap.Sign() < 0 {
		return nil, fmt.Errorf("failed to create unsigned evm dynamic fee transaction: gas_tip_cap=out_of_range")
	}
	if params.GasFeeCap == nil {
		return nil, fmt.Errorf("failed to create unsigned evm dynamic fee transaction: gas_fee_cap=null")
	}
	if params.GasFeeCap.Sign() < 0 || params.GasFeeCap.Cmp(params.GasTipCap) < 0 {
		return nil, fmt.Errorf("failed to create unsigned evm dynamic fee transaction: gas_fee_cap=out_of_range")
	}
	if params.GasLimit == 0 {
		return nil, fmt.Errorf("failed to create unsigned evm dynamic fee transaction: gas_limit=empty")
	}
	if params.To == (common.Address{}) {
		return nil, fmt.Errorf("failed to create unsigned evm dynamic fee transaction: to=empty")
	}
	if params.Value == nil {
		return nil, fmt.Errorf("failed to create unsigned evm dynamic fee transaction: value=null")
	}
	if params.Value.Sign() < 0 {
		return nil, fmt.Errorf("failed to create unsigned evm dynamic fee transaction: value=out_of_range")
	}
	chainID := new(big.Int).SetUint64(uint64(params.ChainID))
	transaction := types.NewTx(&types.DynamicFeeTx{
		ChainID: chainID, Nonce: params.Nonce,
		GasTipCap: new(big.Int).Set(params.GasTipCap), GasFeeCap: new(big.Int).Set(params.GasFeeCap),
		Gas: params.GasLimit, To: &params.To, Value: new(big.Int).Set(params.Value), Data: append([]byte(nil), params.Data...),
	})
	return &UnsignedDynamicFeeTransaction{chainID: chainID, value: transaction}, nil
}

// Transaction returns the underlying unsigned go-ethereum transaction.
//
// Returns:
//   - Unsigned EIP-1559 transaction.
//
// Version:
//   - 2026-09-05: Added.
func (t *UnsignedDynamicFeeTransaction) Transaction() (*types.Transaction, error) {
	if t == nil || t.value == nil {
		return nil, fmt.Errorf("failed to get unsigned evm dynamic fee transaction: transaction=null")
	}
	return t.value, nil
}

// SigningHash returns the EIP-155 signing hash for the unsigned transaction.
//
// Returns:
//   - Hash that the configured chain signer must authorize.
//   - Hashing error.
//
// Version:
//   - 2026-09-05: Added.
func (t *UnsignedDynamicFeeTransaction) SigningHash() (common.Hash, error) {
	if t == nil || t.value == nil || t.chainID == nil {
		return common.Hash{}, fmt.Errorf("failed to hash unsigned evm dynamic fee transaction: transaction=null")
	}
	return types.LatestSignerForChainID(t.chainID).Hash(t.value), nil
}

// MarshalBinary serializes the unsigned EIP-1559 transaction envelope.
//
// Returns:
//   - EIP-2718 transaction bytes.
//   - Serialization error.
//
// Version:
//   - 2026-09-05: Added.
func (t *UnsignedDynamicFeeTransaction) MarshalBinary() ([]byte, error) {
	if t == nil || t.value == nil {
		return nil, fmt.Errorf("failed to serialize unsigned evm dynamic fee transaction: transaction=null")
	}
	encoded, err := t.value.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize unsigned evm dynamic fee transaction: %w", err)
	}
	return encoded, nil
}
