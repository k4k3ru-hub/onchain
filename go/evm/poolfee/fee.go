// Package poolfee defines observed Pool swap rates in parts per million.
package poolfee

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var ErrUnsupported = errors.New("unsupported pool fee configuration")

type Rates struct {
	Token0ToToken1 uint32
	Token1ToToken0 uint32
}

type ContractReader interface {
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
}

// RateString formats a PPM rate as a normalized decimal fraction without floats.
//
// Version:
//   - 2026-09-22: Added.
func RateString(ppm uint32) (string, error) {
	if ppm > 1_000_000 {
		return "", fmt.Errorf("failed to format pool fee: rate=out_of_range")
	}
	if ppm == 1_000_000 {
		return "1", nil
	}
	s := strings.TrimRight(fmt.Sprintf("0.%06d", ppm), "0")
	return strings.TrimRight(s, "."), nil
}

// Call reads one getter at an explicit block and caller without retries or caching.
// Callers own canonical block verification, scheduling and acquisition budgets.
//
// Version:
//   - 2026-09-22: Added.
func Call(ctx context.Context, rpc ContractReader, target, sender common.Address, block *big.Int, signature string, arguments []byte) ([]byte, error) {
	if ctx == nil || rpc == nil || block == nil {
		return nil, fmt.Errorf("failed to read pool fee: dependency=null")
	}
	if block.Sign() < 0 || target == (common.Address{}) {
		return nil, fmt.Errorf("failed to read pool fee: request=invalid")
	}
	data := append(append([]byte(nil), crypto.Keccak256([]byte(signature))[:4]...), arguments...)
	v, err := rpc.CallContract(ctx, ethereum.CallMsg{To: &target, From: sender, Gas: 1_000_000, Data: data}, block)
	if err != nil {
		return nil, fmt.Errorf("failed to read pool fee: %w: method=%q", err, signature)
	}
	return v, nil
}

// UintWord decodes one canonical ABI unsigned integer within the supplied bound.
//
// Version:
//   - 2026-09-22: Added.
func UintWord(raw []byte, maximum uint64) (uint64, error) {
	if len(raw) != 32 {
		return 0, fmt.Errorf("failed to decode pool fee: data=invalid")
	}
	n := new(big.Int).SetBytes(raw)
	if !n.IsUint64() || n.Uint64() > maximum {
		return 0, fmt.Errorf("failed to decode pool fee: value=out_of_range")
	}
	return n.Uint64(), nil
}

// AddressWord decodes one canonical ABI address word.
//
// Version:
//   - 2026-09-22: Added.
func AddressWord(raw []byte) (common.Address, error) {
	if len(raw) != 32 || new(big.Int).SetBytes(raw).BitLen() > 160 {
		return common.Address{}, fmt.Errorf("failed to decode pool fee address: data=invalid")
	}
	return common.BytesToAddress(raw), nil
}
