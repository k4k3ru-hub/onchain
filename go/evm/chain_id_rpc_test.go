package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"
)

var errTestChainID = errors.New("test chain id error")

type fakeChainIDProvider struct {
	ctx     context.Context
	chainID *big.Int
	err     error
}

func (f *fakeChainIDProvider) ChainID(ctx context.Context) (*big.Int, error) {
	f.ctx = ctx
	return f.chainID, f.err
}

func TestHTTPClientChainID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		chainID uint64
	}{
		{name: "ethereum mainnet", chainID: 1},
		{name: "bnb mainnet", chainID: 56},
		{name: "base mainnet", chainID: 8453},
		{name: "unknown evm chain", chainID: 999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			type contextKey string
			ctx := context.WithValue(context.Background(), contextKey("key"), "value")
			provider := &fakeChainIDProvider{chainID: new(big.Int).SetUint64(tt.chainID)}
			client := &HTTPClient{chainIDProvider: provider}

			got, err := client.ChainID(ctx)
			if err != nil {
				t.Fatalf("ChainID() error = %v", err)
			}
			if got.Uint64() != tt.chainID {
				t.Errorf("ChainID() = %d, want %d", got, tt.chainID)
			}
			if provider.ctx != ctx {
				t.Fatal("ChainID() delegated context differs from input")
			}
		})
	}
}

func TestHTTPClientChainIDHandlesNilContext(t *testing.T) {
	t.Parallel()

	provider := &fakeChainIDProvider{chainID: big.NewInt(1)}
	client := &HTTPClient{chainIDProvider: provider}

	_, err := client.ChainID(nil)
	if err != nil {
		t.Fatalf("ChainID() error = %v", err)
	}
	if provider.ctx == nil {
		t.Fatal("ChainID() delegated context = nil, want context.Background()")
	}
}

func TestHTTPClientChainIDRejectsInvalidState(t *testing.T) {
	t.Parallel()

	overflow := new(big.Int).Lsh(big.NewInt(1), 64)
	tests := []struct {
		name      string
		client    *HTTPClient
		wantError string
	}{
		{name: "nil client", wantError: "failed to get evm chain id: http_client=null"},
		{name: "nil provider", client: &HTTPClient{}, wantError: "failed to get evm chain id: chain_id_provider=null"},
		{name: "nil chain id", client: &HTTPClient{chainIDProvider: &fakeChainIDProvider{}}, wantError: "failed to get evm chain id: chain_id=null"},
		{name: "negative chain id", client: &HTTPClient{chainIDProvider: &fakeChainIDProvider{chainID: big.NewInt(-1)}}, wantError: "failed to get evm chain id: chain_id=out_of_range"},
		{name: "overflow chain id", client: &HTTPClient{chainIDProvider: &fakeChainIDProvider{chainID: overflow}}, wantError: "failed to get evm chain id: chain_id=out_of_range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.client.ChainID(context.Background())
			if err == nil {
				t.Fatal("ChainID() error = nil, want error")
			}
			if err.Error() != tt.wantError {
				t.Errorf("ChainID() error = %q, want %q", err, tt.wantError)
			}
		})
	}
}

func TestHTTPClientChainIDWrapsProviderError(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{chainIDProvider: &fakeChainIDProvider{err: errTestChainID}}
	_, err := client.ChainID(context.Background())
	if !errors.Is(err, errTestChainID) {
		t.Fatalf("ChainID() error = %v, want wrapped provider error", err)
	}
	if err.Error() != "failed to get evm chain id: test chain id error" {
		t.Errorf("ChainID() error = %q", err)
	}
}

func TestWSClientChainID(t *testing.T) {
	t.Parallel()

	provider := &fakeChainIDProvider{chainID: big.NewInt(11155111)}
	client := &WSClient{chainIDProvider: provider}

	got, err := client.ChainID(context.Background())
	if err != nil {
		t.Fatalf("ChainID() error = %v", err)
	}
	if got != ChainIDEthereumSepolia {
		t.Errorf("ChainID() = %d, want %d", got, ChainIDEthereumSepolia)
	}
}

func TestWSClientChainIDRejectsInvalidState(t *testing.T) {
	t.Parallel()

	var nilClient *WSClient
	_, err := nilClient.ChainID(context.Background())
	if err == nil || err.Error() != "failed to get evm chain id: ws_client=null" {
		t.Fatalf("ChainID() error = %v", err)
	}

	_, err = (&WSClient{}).ChainID(context.Background())
	if err == nil || err.Error() != "failed to get evm chain id: chain_id_provider=null" {
		t.Fatalf("ChainID() error = %v", err)
	}
}
