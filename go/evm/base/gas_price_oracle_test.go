package base

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

type gasPriceOracleCaller struct {
	result []byte
	err    error
	call   ethereum.CallMsg
}

func (c *gasPriceOracleCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	c.call = call
	return c.result, c.err
}

func TestGasPriceOracleGetL1Fee(t *testing.T) {
	address := common.HexToAddress("0x420000000000000000000000000000000000000F")
	caller := &gasPriceOracleCaller{}
	oracle, err := NewGasPriceOracle(caller, address)
	if err != nil {
		t.Fatal(err)
	}
	caller.result, err = oracle.abi.Methods["getL1Fee"].Outputs.Pack(big.NewInt(12345))
	if err != nil {
		t.Fatal(err)
	}
	fee, err := oracle.GetL1Fee(t.Context(), []byte{1, 2, 3}, nil)
	if err != nil {
		t.Fatalf("GetL1Fee() error = %v", err)
	}
	if fee.Cmp(big.NewInt(12345)) != 0 {
		t.Fatalf("GetL1Fee() = %s, want 12345", fee)
	}
	if caller.call.To == nil || *caller.call.To != address || len(caller.call.Data) <= 4 {
		t.Fatalf("CallContract() call = %+v", caller.call)
	}
}

func TestGasPriceOracleValidationAndPropagation(t *testing.T) {
	address := common.HexToAddress("0x420000000000000000000000000000000000000F")
	if _, err := NewGasPriceOracle(nil, address); err == nil {
		t.Fatal("NewGasPriceOracle() error = nil")
	}
	callErr := errors.New("rpc unavailable")
	oracle, err := NewGasPriceOracle(&gasPriceOracleCaller{err: callErr}, address)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oracle.GetL1Fee(t.Context(), []byte{1}, nil); !errors.Is(err, callErr) {
		t.Fatalf("GetL1Fee() error = %v, want wrapped error", err)
	}
	if _, err := oracle.GetL1Fee(t.Context(), nil, nil); err == nil {
		t.Fatal("GetL1Fee() empty transaction error = nil")
	}
}

func TestGasPriceOracleGetL1FeeUpperBound(t *testing.T) {
	address := common.HexToAddress(GasPriceOraclePredeployAddress)
	caller := &gasPriceOracleCaller{}
	oracle, err := NewGasPriceOracle(caller, address)
	if err != nil {
		t.Fatal(err)
	}
	caller.result, err = oracle.abi.Methods["getL1FeeUpperBound"].Outputs.Pack(big.NewInt(45678))
	if err != nil {
		t.Fatal(err)
	}
	fee, err := oracle.GetL1FeeUpperBound(t.Context(), 256, nil)
	if err != nil {
		t.Fatalf("GetL1FeeUpperBound() error = %v", err)
	}
	if fee.Cmp(big.NewInt(45678)) != 0 {
		t.Fatalf("GetL1FeeUpperBound() = %s, want 45678", fee)
	}
	if _, err := oracle.GetL1FeeUpperBound(t.Context(), 0, nil); err == nil {
		t.Fatal("GetL1FeeUpperBound() empty size error = nil")
	}
}
