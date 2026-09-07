package protocol

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const MaxFee uint32 = 0xffffff

type PoolKey struct {
	Token0 Currency
	Token1 Currency
	Fee    uint32
}

// Validate validates a Uniswap v3 pool key.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-24: Added.
func (k PoolKey) Validate() error {
	token0 := k.Token0.Address()
	token1 := k.Token1.Address()
	if token0 == (common.Address{}) {
		return fmt.Errorf("failed to validate uniswap v3 pool key: token0=empty")
	}
	if token1 == (common.Address{}) {
		return fmt.Errorf("failed to validate uniswap v3 pool key: token1=empty")
	}
	if bytes.Compare(token0[:], token1[:]) >= 0 {
		return fmt.Errorf("failed to validate uniswap v3 pool key: token_order=invalid")
	}
	if k.Fee == 0 || k.Fee > MaxFee {
		return fmt.Errorf("failed to validate uniswap v3 pool key: fee=out_of_range min_value=1 max_value=%d", MaxFee)
	}
	return nil
}

// Address calculates the deterministic Uniswap v3 pool address.
//
// Parameters:
//   - factory: Uniswap v3 factory address.
//   - initCodeHash: Uniswap v3 pool init code hash.
//
// Returns:
//   - Deterministic pool address.
//   - Calculation error.
//
// Version:
//   - 2026-08-24: Added.
func (k PoolKey) Address(factory common.Address, initCodeHash common.Hash) (common.Address, error) {
	if err := k.Validate(); err != nil {
		return common.Address{}, fmt.Errorf("failed to calculate uniswap v3 pool address: %w", err)
	}
	if factory == (common.Address{}) {
		return common.Address{}, fmt.Errorf("failed to calculate uniswap v3 pool address: factory=empty")
	}
	if initCodeHash == (common.Hash{}) {
		return common.Address{}, fmt.Errorf("failed to calculate uniswap v3 pool address: init_code_hash=empty")
	}

	addressType, err := abi.NewType("address", "", nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to calculate uniswap v3 pool address: %w", err)
	}
	uint24Type, err := abi.NewType("uint24", "", nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to calculate uniswap v3 pool address: %w", err)
	}
	encoded, err := (abi.Arguments{{Type: addressType}, {Type: addressType}, {Type: uint24Type}}).Pack(
		k.Token0.Address(), k.Token1.Address(), new(big.Int).SetUint64(uint64(k.Fee)),
	)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to calculate uniswap v3 pool address: %w", err)
	}
	salt := crypto.Keccak256Hash(encoded)
	return crypto.CreateAddress2(factory, salt, initCodeHash[:]), nil
}
