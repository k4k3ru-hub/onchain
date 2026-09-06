package base

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const gasPriceOracleABI = `[{"inputs":[{"internalType":"bytes","name":"_data","type":"bytes"}],"name":"getL1Fee","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint256","name":"_unsignedTxSize","type":"uint256"}],"name":"getL1FeeUpperBound","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}]`

const GasPriceOraclePredeployAddress = "0x420000000000000000000000000000000000000F"

type ContractCaller interface {
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
}

type GasPriceOracle struct {
	caller  ContractCaller
	address common.Address
	abi     abi.ABI
}

// NewGasPriceOracle creates a Base GasPriceOracle client.
//
// Parameters:
//   - caller: EVM contract-call dependency.
//   - address: Base GasPriceOracle predeploy address.
//
// Returns:
//   - GasPriceOracle client.
//   - Creation error.
//
// Version:
//   - 2026-09-06: Added.
func NewGasPriceOracle(caller ContractCaller, address common.Address) (*GasPriceOracle, error) {
	if caller == nil {
		return nil, fmt.Errorf("failed to create base gas price oracle: caller=null")
	}
	if address == (common.Address{}) {
		return nil, fmt.Errorf("failed to create base gas price oracle: address=empty")
	}
	parsed, err := abi.JSON(strings.NewReader(gasPriceOracleABI))
	if err != nil {
		return nil, fmt.Errorf("failed to create base gas price oracle: failed to parse contract abi: %w", err)
	}
	return &GasPriceOracle{caller: caller, address: address, abi: parsed}, nil
}

// GetL1Fee returns the Base L1 data fee for serialized transaction bytes.
//
// Parameters:
//   - ctx: Contract-call context; nil uses context.Background.
//   - transaction: Serialized unsigned transaction bytes.
//   - blockNumber: Optional block used for the read.
//
// Returns:
//   - L1 data fee in wei.
//   - Contract-call or decoding error.
//
// Version:
//   - 2026-09-06: Added.
func (o *GasPriceOracle) GetL1Fee(ctx context.Context, transaction []byte, blockNumber *big.Int) (*big.Int, error) {
	if o == nil || o.caller == nil {
		return nil, fmt.Errorf("failed to get base l1 fee: gas_price_oracle=null")
	}
	if len(transaction) == 0 {
		return nil, fmt.Errorf("failed to get base l1 fee: transaction=empty")
	}
	if blockNumber != nil && blockNumber.Sign() < 0 {
		return nil, fmt.Errorf("failed to get base l1 fee: block_number=out_of_range")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := o.abi.Pack("getL1Fee", append([]byte(nil), transaction...))
	if err != nil {
		return nil, fmt.Errorf("failed to get base l1 fee: failed to encode contract call: %w", err)
	}
	result, err := o.caller.CallContract(ctx, ethereum.CallMsg{To: &o.address, Data: data}, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get base l1 fee: %w", err)
	}
	values, err := o.abi.Methods["getL1Fee"].Outputs.Unpack(result)
	if err != nil {
		return nil, fmt.Errorf("failed to get base l1 fee: failed to decode contract response: %w", err)
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("failed to get base l1 fee: decoded_values=invalid")
	}
	fee, ok := values[0].(*big.Int)
	if !ok || fee.Sign() < 0 {
		return nil, fmt.Errorf("failed to get base l1 fee: fee=invalid")
	}
	return new(big.Int).Set(fee), nil
}

// GetL1FeeUpperBound returns the Base L1 fee upper bound for an unsigned transaction size.
//
// Parameters:
//   - ctx: Contract-call context; nil uses context.Background.
//   - unsignedTransactionSize: Serialized unsigned transaction size in bytes.
//   - blockNumber: Optional block used for the read.
//
// Returns:
//   - L1 data fee upper bound in wei.
//   - Contract-call or decoding error.
//
// Version:
//   - 2026-09-06: Added.
func (o *GasPriceOracle) GetL1FeeUpperBound(ctx context.Context, unsignedTransactionSize uint64, blockNumber *big.Int) (*big.Int, error) {
	if o == nil || o.caller == nil {
		return nil, fmt.Errorf("failed to get base l1 fee upper bound: gas_price_oracle=null")
	}
	if unsignedTransactionSize == 0 {
		return nil, fmt.Errorf("failed to get base l1 fee upper bound: unsigned_transaction_size=empty")
	}
	if blockNumber != nil && blockNumber.Sign() < 0 {
		return nil, fmt.Errorf("failed to get base l1 fee upper bound: block_number=out_of_range")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := o.abi.Pack("getL1FeeUpperBound", new(big.Int).SetUint64(unsignedTransactionSize))
	if err != nil {
		return nil, fmt.Errorf("failed to get base l1 fee upper bound: failed to encode contract call: %w", err)
	}
	result, err := o.caller.CallContract(ctx, ethereum.CallMsg{To: &o.address, Data: data}, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get base l1 fee upper bound: %w", err)
	}
	values, err := o.abi.Methods["getL1FeeUpperBound"].Outputs.Unpack(result)
	if err != nil {
		return nil, fmt.Errorf("failed to get base l1 fee upper bound: failed to decode contract response: %w", err)
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("failed to get base l1 fee upper bound: decoded_values=invalid")
	}
	fee, ok := values[0].(*big.Int)
	if !ok || fee.Sign() < 0 {
		return nil, fmt.Errorf("failed to get base l1 fee upper bound: fee=invalid")
	}
	return new(big.Int).Set(fee), nil
}
