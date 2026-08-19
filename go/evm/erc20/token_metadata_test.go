package erc20

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type metadataHTTPClient struct {
	responses   map[string][]byte
	err         error
	calls       []ethereum.CallMsg
	contexts    []context.Context
	blockNumber []*big.Int
}

func (f *metadataHTTPClient) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	f.contexts = append(f.contexts, ctx)
	f.calls = append(f.calls, call)
	f.blockNumber = append(f.blockNumber, blockNumber)
	if f.err != nil {
		return nil, f.err
	}
	return f.responses[string(call.Data)], nil
}

func (f *metadataHTTPClient) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

func (f *metadataHTTPClient) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	return nil, nil
}

func TestGetTokenMetadata(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000001")
	httpClient := &metadataHTTPClient{responses: map[string][]byte{
		string(symbolMethodSelector):   packMetadataValue(t, "WBTC", "string"),
		string(decimalsMethodSelector): packMetadataValue(t, uint8(8), "uint8"),
	}}
	client, err := NewClient(httpClient, nil, []common.Address{token})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	blockNumber := big.NewInt(123)
	metadata, err := client.GetTokenMetadata(nil, token, blockNumber)
	if err != nil {
		t.Fatalf("GetTokenMetadata() error = %v", err)
	}
	if metadata.Symbol != "WBTC" {
		t.Errorf("GetTokenMetadata().Symbol = %q, want %q", metadata.Symbol, "WBTC")
	}
	if metadata.Decimals != 8 {
		t.Errorf("GetTokenMetadata().Decimals = %d, want 8", metadata.Decimals)
	}
	if len(httpClient.calls) != 2 {
		t.Fatalf("CallContract() calls = %d, want 2", len(httpClient.calls))
	}
	for i := range httpClient.calls {
		if httpClient.contexts[i] == nil {
			t.Errorf("CallContract() context at index %d = nil", i)
		}
		if httpClient.blockNumber[i] != blockNumber {
			t.Errorf("CallContract() block number at index %d differs from input", i)
		}
		if httpClient.calls[i].To == nil || *httpClient.calls[i].To != token {
			t.Errorf("CallContract() token at index %d = %v, want %s", i, httpClient.calls[i].To, token.Hex())
		}
	}
}

func TestGetTokenSymbolWrapsCallError(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000001")
	callErr := errors.New("rpc unavailable")
	client, err := NewClient(&metadataHTTPClient{err: callErr}, nil, []common.Address{token})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.GetTokenSymbol(context.Background(), token, nil)
	if !errors.Is(err, callErr) {
		t.Fatalf("GetTokenSymbol() error = %v, want wrapped call error", err)
	}
}

func TestGetTokenDecimalsAcceptsBoundaryValues(t *testing.T) {
	tests := []uint8{0, 6, 8, 18, 255}
	for _, want := range tests {
		t.Run(new(big.Int).SetUint64(uint64(want)).String(), func(t *testing.T) {
			token := common.HexToAddress("0x0000000000000000000000000000000000000001")
			httpClient := &metadataHTTPClient{responses: map[string][]byte{
				string(decimalsMethodSelector): packMetadataValue(t, want, "uint8"),
			}}
			client, err := NewClient(httpClient, nil, []common.Address{token})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			got, err := client.GetTokenDecimals(context.Background(), token, nil)
			if err != nil {
				t.Fatalf("GetTokenDecimals() error = %v", err)
			}
			if got != want {
				t.Errorf("GetTokenDecimals() = %d, want %d", got, want)
			}
		})
	}
}

func TestGetTokenMetadataRejectsInvalidInput(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000001")
	otherToken := common.HexToAddress("0x0000000000000000000000000000000000000002")
	client, err := NewClient(&metadataHTTPClient{}, nil, []common.Address{token})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	tests := []struct {
		name        string
		client      *Client
		token       common.Address
		blockNumber *big.Int
	}{
		{name: "nil client", client: nil, token: token},
		{name: "empty token", client: client},
		{name: "unconfigured token", client: client, token: otherToken},
		{name: "negative block", client: client, token: token, blockNumber: big.NewInt(-1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.client.GetTokenMetadata(context.Background(), tt.token, tt.blockNumber)
			if err == nil {
				t.Fatal("GetTokenMetadata() error = nil, want error")
			}
		})
	}
}

func TestGetTokenSymbolRejectsInvalidResponse(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000001")
	tests := []struct {
		name     string
		response []byte
	}{
		{name: "empty response"},
		{name: "malformed response", response: []byte{1}},
		{name: "empty symbol", response: packMetadataValue(t, "", "string")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient := &metadataHTTPClient{responses: map[string][]byte{
				string(symbolMethodSelector): tt.response,
			}}
			client, err := NewClient(httpClient, nil, []common.Address{token})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			_, err = client.GetTokenSymbol(context.Background(), token, nil)
			if err == nil {
				t.Fatal("GetTokenSymbol() error = nil, want error")
			}
		})
	}
}

func TestGetTokenDecimalsRejectsMalformedResponse(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000001")
	httpClient := &metadataHTTPClient{responses: map[string][]byte{
		string(decimalsMethodSelector): {1},
	}}
	client, err := NewClient(httpClient, nil, []common.Address{token})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.GetTokenDecimals(context.Background(), token, nil)
	if err == nil {
		t.Fatal("GetTokenDecimals() error = nil, want error")
	}
}

func packMetadataValue(t *testing.T, value any, abiType string) []byte {
	t.Helper()

	typ, err := abi.NewType(abiType, "", nil)
	if err != nil {
		t.Fatalf("abi.NewType() error = %v", err)
	}
	result, err := (abi.Arguments{{Type: typ}}).Pack(value)
	if err != nil {
		t.Fatalf("abi.Arguments.Pack() error = %v", err)
	}
	return result
}
