package protocol

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	myOnchainEVM "github.com/k4k3ru-hub/onchain/go/evm"
)

type Currency common.Address

// NewCurrency creates a Uniswap v3 ERC-20 currency.
//
// Parameters:
//   - address: ERC-20 contract address.
//
// Returns:
//   - Uniswap v3 currency.
//
// Version:
//   - 2026-08-24: Added.
func NewCurrency(address common.Address) Currency { return Currency(address) }

// ParseCurrency parses an EVM address as a Uniswap v3 ERC-20 currency.
//
// Parameters:
//   - value: ERC-20 contract address.
//
// Returns:
//   - Parsed currency.
//   - Parse error.
//
// Version:
//   - 2026-08-24: Added.
func ParseCurrency(value string) (Currency, error) {
	address, err := myOnchainEVM.ParseAddress(value)
	if err != nil {
		return Currency{}, fmt.Errorf("failed to parse uniswap v3 currency: %w", err)
	}
	if address == (common.Address{}) {
		return Currency{}, fmt.Errorf("failed to parse uniswap v3 currency: address=empty")
	}
	return NewCurrency(address), nil
}

// Address returns the underlying ERC-20 address.
//
// Returns:
//   - ERC-20 contract address.
//
// Version:
//   - 2026-08-24: Added.
func (c Currency) Address() common.Address { return common.Address(c) }

// Hex returns the checksummed hexadecimal address.
//
// Returns:
//   - Checksummed hexadecimal address.
//
// Version:
//   - 2026-08-24: Added.
func (c Currency) Hex() string { return c.Address().Hex() }
