package evm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestNewUnsignedDynamicFeeTransaction(t *testing.T) {
	t.Parallel()
	to := common.HexToAddress("0x0000000000000000000000000000000000000001")
	transaction, err := NewUnsignedDynamicFeeTransaction(DynamicFeeTransactionParams{
		ChainID: 8453, Nonce: 7, GasTipCap: big.NewInt(2), GasFeeCap: big.NewInt(20),
		GasLimit: 123, To: to, Value: big.NewInt(3), Data: []byte{0xde, 0xad},
	})
	if err != nil {
		t.Fatalf("NewUnsignedDynamicFeeTransaction() error = %v", err)
	}
	value, err := transaction.Transaction()
	if err != nil {
		t.Fatalf("Transaction() error = %v", err)
	}
	if value.Type() != types.DynamicFeeTxType || value.ChainId().Uint64() != 8453 || value.Nonce() != 7 || value.Gas() != 123 || *value.To() != to {
		t.Fatalf("Transaction() = %+v", value)
	}
	hash, err := transaction.SigningHash()
	if err != nil {
		t.Fatalf("SigningHash() error = %v", err)
	}
	wantHash := types.LatestSignerForChainID(big.NewInt(8453)).Hash(value)
	if hash != wantHash || hash == (common.Hash{}) {
		t.Fatalf("SigningHash() = %s, want %s", hash, wantHash)
	}
	encoded, err := transaction.MarshalBinary()
	if err != nil || len(encoded) == 0 {
		t.Fatalf("MarshalBinary() = %x, %v", encoded, err)
	}
}

func TestNewUnsignedDynamicFeeTransactionCopiesMutableInputs(t *testing.T) {
	t.Parallel()
	tip, fee, value, data := big.NewInt(2), big.NewInt(20), big.NewInt(3), []byte{1}
	transaction, err := NewUnsignedDynamicFeeTransaction(DynamicFeeTransactionParams{
		ChainID: 8453, GasTipCap: tip, GasFeeCap: fee, GasLimit: 1,
		To: common.HexToAddress("0x0000000000000000000000000000000000000001"), Value: value, Data: data,
	})
	if err != nil {
		t.Fatal(err)
	}
	tip.SetInt64(9)
	fee.SetInt64(9)
	value.SetInt64(9)
	data[0] = 9
	built, err := transaction.Transaction()
	if err != nil {
		t.Fatal(err)
	}
	if built.GasTipCap().Int64() != 2 || built.GasFeeCap().Int64() != 20 || built.Value().Int64() != 3 || built.Data()[0] != 1 {
		t.Fatalf("Transaction() aliases inputs = %+v", built)
	}
}

func TestNewUnsignedDynamicFeeTransactionValidatesParams(t *testing.T) {
	t.Parallel()
	valid := DynamicFeeTransactionParams{
		ChainID: 1, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2), GasLimit: 1,
		To: common.HexToAddress("0x0000000000000000000000000000000000000001"), Value: new(big.Int), Data: []byte{1},
	}
	tests := []struct {
		name   string
		mutate func(*DynamicFeeTransactionParams)
	}{
		{name: "chain", mutate: func(p *DynamicFeeTransactionParams) { p.ChainID = 0 }},
		{name: "tip", mutate: func(p *DynamicFeeTransactionParams) { p.GasTipCap = nil }},
		{name: "fee below tip", mutate: func(p *DynamicFeeTransactionParams) { p.GasFeeCap = new(big.Int) }},
		{name: "gas", mutate: func(p *DynamicFeeTransactionParams) { p.GasLimit = 0 }},
		{name: "to", mutate: func(p *DynamicFeeTransactionParams) { p.To = common.Address{} }},
		{name: "value", mutate: func(p *DynamicFeeTransactionParams) { p.Value = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := valid
			test.mutate(&params)
			if _, err := NewUnsignedDynamicFeeTransaction(params); err == nil {
				t.Fatal("NewUnsignedDynamicFeeTransaction() error = nil")
			}
		})
	}
}
