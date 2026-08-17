package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var errTestHeaderByHash = errors.New("test header by hash error")

type fakeBlockHeaderByHasher struct {
	header *types.Header
	err    error
	ctx    context.Context
	hash   common.Hash
}

func (f *fakeBlockHeaderByHasher) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	f.ctx = ctx
	f.hash = hash
	return f.header, f.err
}

func TestHeaderByHashDelegatesAndConvertsHeader(t *testing.T) {
	t.Parallel()

	header := &types.Header{
		Number:     big.NewInt(123),
		ParentHash: common.HexToHash("0x1234"),
		Time:       1_700_000_000,
	}
	blockHash := header.Hash()
	requestContext := context.WithValue(context.Background(), struct{}{}, "test")
	provider := &fakeBlockHeaderByHasher{header: header}
	client := &HTTPClient{blockHeaderByHasher: provider}

	got, err := client.HeaderByHash(requestContext, blockHash)
	if err != nil {
		t.Fatalf("HeaderByHash() error = %v", err)
	}
	if provider.ctx != requestContext {
		t.Fatal("HeaderByHash() delegated context differs from input")
	}
	if provider.hash != blockHash {
		t.Errorf("HeaderByHash() delegated hash = %v, want %v", provider.hash, blockHash)
	}
	if got.Number != 123 {
		t.Errorf("HeaderByHash() Number = %d, want 123", got.Number)
	}
	if got.Hash != blockHash {
		t.Errorf("HeaderByHash() Hash = %v, want %v", got.Hash, blockHash)
	}
	if got.ParentHash != header.ParentHash {
		t.Errorf("HeaderByHash() ParentHash = %v, want %v", got.ParentHash, header.ParentHash)
	}
	if got.Timestamp != header.Time {
		t.Errorf("HeaderByHash() Timestamp = %d, want %d", got.Timestamp, header.Time)
	}
}

func TestHeaderByHashUsesBackgroundContextForNilContext(t *testing.T) {
	t.Parallel()

	header := &types.Header{Number: big.NewInt(1)}
	blockHash := header.Hash()
	provider := &fakeBlockHeaderByHasher{header: header}
	client := &HTTPClient{blockHeaderByHasher: provider}

	if _, err := client.HeaderByHash(nil, blockHash); err != nil {
		t.Fatalf("HeaderByHash() error = %v", err)
	}
	if provider.ctx == nil {
		t.Fatal("HeaderByHash() delegated context = nil, want non-nil context")
	}
}

func TestHeaderByHashRejectsInvalidState(t *testing.T) {
	t.Parallel()

	validHeader := &types.Header{Number: big.NewInt(1)}
	validHash := validHeader.Hash()
	overflowNumber := new(big.Int).Lsh(big.NewInt(1), 64)

	tests := []struct {
		name      string
		client    *HTTPClient
		blockHash common.Hash
		wantError string
	}{
		{
			name:      "nil receiver",
			client:    nil,
			blockHash: validHash,
			wantError: "failed to get evm header by hash: evm_http_client=null",
		},
		{
			name:      "missing dependency",
			client:    &HTTPClient{},
			blockHash: validHash,
			wantError: "failed to get evm header by hash: http_eth_client=null",
		},
		{
			name:      "zero block hash",
			client:    &HTTPClient{blockHeaderByHasher: &fakeBlockHeaderByHasher{}},
			blockHash: common.Hash{},
			wantError: "failed to get evm header by hash: block_hash=empty",
		},
		{
			name:      "nil header",
			client:    &HTTPClient{blockHeaderByHasher: &fakeBlockHeaderByHasher{}},
			blockHash: validHash,
			wantError: "failed to get evm header by hash: block_header=null",
		},
		{
			name:      "nil block number",
			client:    &HTTPClient{blockHeaderByHasher: &fakeBlockHeaderByHasher{header: &types.Header{}}},
			blockHash: validHash,
			wantError: "failed to get evm header by hash: block_number=null",
		},
		{
			name:      "negative block number",
			client:    &HTTPClient{blockHeaderByHasher: &fakeBlockHeaderByHasher{header: &types.Header{Number: big.NewInt(-1)}}},
			blockHash: validHash,
			wantError: "failed to get evm header by hash: block_number=out_of_range",
		},
		{
			name:      "overflow block number",
			client:    &HTTPClient{blockHeaderByHasher: &fakeBlockHeaderByHasher{header: &types.Header{Number: overflowNumber}}},
			blockHash: validHash,
			wantError: "failed to get evm header by hash: block_number=out_of_range",
		},
		{
			name:      "hash mismatch",
			client:    &HTTPClient{blockHeaderByHasher: &fakeBlockHeaderByHasher{header: validHeader}},
			blockHash: common.HexToHash("0x1234"),
			wantError: "failed to get evm header by hash: block_hash=mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.client.HeaderByHash(context.Background(), tt.blockHash)
			if err == nil {
				t.Fatal("HeaderByHash() error = nil, want error")
			}
			if err.Error() != tt.wantError {
				t.Errorf("HeaderByHash() error = %q, want %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestHeaderByHashWrapsRPCError(t *testing.T) {
	t.Parallel()

	header := &types.Header{Number: big.NewInt(1)}
	client := &HTTPClient{blockHeaderByHasher: &fakeBlockHeaderByHasher{err: errTestHeaderByHash}}

	_, err := client.HeaderByHash(context.Background(), header.Hash())
	if err == nil {
		t.Fatal("HeaderByHash() error = nil, want error")
	}
	if err.Error() != "failed to get evm header by hash: test header by hash error" {
		t.Errorf("HeaderByHash() error = %q", err.Error())
	}
	if !errors.Is(err, errTestHeaderByHash) {
		t.Fatalf("errors.Is() = false, want wrapped error: %v", err)
	}
}

func TestComposeHTTPClientComposesBlockHeaderByHasher(t *testing.T) {
	t.Parallel()

	ethClient := new(ethclient.Client)
	client := composeHTTPClient(HTTPConfig{}, ethClient)

	if client.blockHeaderByHasher != ethClient {
		t.Fatal("HTTP block-header dependency was not composed")
	}
}
