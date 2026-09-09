package erc20

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestGetTokenBalanceAndAllowance(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000001")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000002")
	spender := common.HexToAddress("0x0000000000000000000000000000000000000003")
	httpClient := &metadataHTTPClient{responses: map[string][]byte{}}
	balanceData := append(append([]byte(nil), balanceOfMethodSelector...), common.LeftPadBytes(owner.Bytes(), 32)...)
	allowanceData := append(append([]byte(nil), allowanceMethodSelector...), common.LeftPadBytes(owner.Bytes(), 32)...)
	allowanceData = append(allowanceData, common.LeftPadBytes(spender.Bytes(), 32)...)
	httpClient.responses[string(balanceData)] = common.LeftPadBytes(big.NewInt(21_000_000).Bytes(), 32)
	httpClient.responses[string(allowanceData)] = common.LeftPadBytes(big.NewInt(10_000).Bytes(), 32)
	client, err := NewClient(httpClient, nil, []common.Address{token})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}

	balance, err := client.GetTokenBalance(t.Context(), token, owner, nil)
	if err != nil || balance.Cmp(big.NewInt(21_000_000)) != 0 {
		t.Fatalf("GetTokenBalance() = %v, %v", balance, err)
	}
	allowance, err := client.GetTokenAllowance(t.Context(), token, owner, spender, nil)
	if err != nil || allowance.Cmp(big.NewInt(10_000)) != 0 {
		t.Fatalf("GetTokenAllowance() = %v, %v", allowance, err)
	}
	if len(httpClient.calls) != 2 || httpClient.calls[0].To == nil || *httpClient.calls[0].To != token {
		t.Fatalf("CallContract() calls = %+v", httpClient.calls)
	}
}

func TestGetTokenAllowanceWrapsCallError(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000001")
	callErr := errors.New("rpc unavailable")
	client, err := NewClient(&metadataHTTPClient{err: callErr}, nil, []common.Address{token})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}
	_, err = client.GetTokenAllowance(context.Background(), token, common.HexToAddress("0x02"), common.HexToAddress("0x03"), nil)
	if !errors.Is(err, callErr) {
		t.Fatalf("GetTokenAllowance() error = %v, want wrapped call error", err)
	}
}

func TestEncodeApprove(t *testing.T) {
	spender := common.HexToAddress("0x0000000000000000000000000000000000000003")
	data, err := EncodeApprove(spender, big.NewInt(10_000))
	if err != nil {
		t.Fatalf("EncodeApprove() error = %v, want nil", err)
	}
	if len(data) != 68 || string(data[:4]) != string(approveMethodSelector) {
		t.Fatalf("EncodeApprove() data = %x", data)
	}
	if common.BytesToAddress(data[4:36]) != spender || new(big.Int).SetBytes(data[36:]).Cmp(big.NewInt(10_000)) != 0 {
		t.Fatalf("EncodeApprove() arguments = %x", data[4:])
	}
}

func TestAllowancePrimitivesRejectInvalidInput(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000001")
	client, err := NewClient(&metadataHTTPClient{}, nil, []common.Address{token})
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}
	tests := []struct {
		name string
		call func() error
	}{
		{name: "empty balance owner", call: func() error { _, err := client.GetTokenBalance(t.Context(), token, common.Address{}, nil); return err }},
		{name: "empty allowance spender", call: func() error {
			_, err := client.GetTokenAllowance(t.Context(), token, common.HexToAddress("0x02"), common.Address{}, nil)
			return err
		}},
		{name: "nil approve amount", call: func() error { _, err := EncodeApprove(common.HexToAddress("0x03"), nil); return err }},
		{name: "negative approve amount", call: func() error { _, err := EncodeApprove(common.HexToAddress("0x03"), big.NewInt(-1)); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("operation error = nil")
			}
		})
	}
}
