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

var (
	errTestHeaderByHash   = errors.New("test header by hash error")
	errTestHeaderByNumber = errors.New("test header by number error")
)

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

type fakeBlockHeaderByNumberer struct {
	header *types.Header
	err    error
	ctx    context.Context
	number *big.Int
}

func (f *fakeBlockHeaderByNumberer) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	f.ctx = ctx
	if number != nil {
		f.number = new(big.Int).Set(number)
	}
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
	if client.blockHeaderByNumberer != ethClient {
		t.Fatal("HTTP block-header-by-number dependency was not composed")
	}
}

func TestHeaderByNumberDelegatesAndConvertsHeader(t *testing.T) {
	t.Parallel()

	header := &types.Header{Number: big.NewInt(123), ParentHash: common.HexToHash("0x1234"), Time: 1_700_000_000}
	requestContext := context.WithValue(context.Background(), struct{}{}, "test")
	provider := &fakeBlockHeaderByNumberer{header: header}
	client := &HTTPClient{blockHeaderByNumberer: provider}

	got, err := client.HeaderByNumber(requestContext, 123)
	if err != nil {
		t.Fatalf("HeaderByNumber() error = %v", err)
	}
	if provider.ctx != requestContext {
		t.Fatal("HeaderByNumber() delegated context differs from input")
	}
	if provider.number == nil || provider.number.Uint64() != 123 {
		t.Fatalf("HeaderByNumber() delegated number = %v, want 123", provider.number)
	}
	if got.Number != 123 || got.Hash != header.Hash() || got.ParentHash != header.ParentHash || got.Timestamp != header.Time {
		t.Fatalf("HeaderByNumber() = %+v, want converted header", got)
	}
}

func TestHeaderByNumberUsesBackgroundContextForNilContext(t *testing.T) {
	t.Parallel()

	provider := &fakeBlockHeaderByNumberer{header: &types.Header{Number: new(big.Int)}}
	client := &HTTPClient{blockHeaderByNumberer: provider}

	if _, err := client.HeaderByNumber(nil, 0); err != nil {
		t.Fatalf("HeaderByNumber() error = %v", err)
	}
	if provider.ctx == nil {
		t.Fatal("HeaderByNumber() delegated context = nil, want non-nil context")
	}
}

func TestHeaderByNumberRejectsInvalidState(t *testing.T) {
	t.Parallel()

	overflowNumber := new(big.Int).Lsh(big.NewInt(1), 64)
	tests := []struct {
		name      string
		client    *HTTPClient
		wantError string
	}{
		{name: "nil receiver", client: nil, wantError: "failed to get evm header by number: evm_http_client=null"},
		{name: "missing dependency", client: &HTTPClient{}, wantError: "failed to get evm header by number: http_eth_client=null"},
		{name: "nil header", client: &HTTPClient{blockHeaderByNumberer: &fakeBlockHeaderByNumberer{}}, wantError: "failed to get evm header by number: block_header=null"},
		{name: "nil returned block number", client: &HTTPClient{blockHeaderByNumberer: &fakeBlockHeaderByNumberer{header: &types.Header{}}}, wantError: "failed to get evm header by number: block_number=null"},
		{name: "overflow returned block number", client: &HTTPClient{blockHeaderByNumberer: &fakeBlockHeaderByNumberer{header: &types.Header{Number: overflowNumber}}}, wantError: "failed to get evm header by number: block_number=out_of_range"},
		{name: "number mismatch", client: &HTTPClient{blockHeaderByNumberer: &fakeBlockHeaderByNumberer{header: &types.Header{Number: big.NewInt(124)}}}, wantError: "failed to get evm header by number: block_number=mismatch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.client.HeaderByNumber(context.Background(), 123)
			if err == nil || err.Error() != tt.wantError {
				t.Fatalf("HeaderByNumber() error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestHeaderByNumberWrapsRPCError(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{blockHeaderByNumberer: &fakeBlockHeaderByNumberer{err: errTestHeaderByNumber}}
	_, err := client.HeaderByNumber(context.Background(), 1)
	if err == nil || !errors.Is(err, errTestHeaderByNumber) {
		t.Fatalf("HeaderByNumber() error = %v, want wrapped error", err)
	}
}

func TestLatestHeaderDelegatesNilNumberAndConvertsHeader(t *testing.T) {
	t.Parallel()

	header := &types.Header{Number: big.NewInt(456), ParentHash: common.HexToHash("0x4567"), Time: 1_800_000_000}
	requestContext := context.WithValue(context.Background(), struct{}{}, "test")
	provider := &fakeBlockHeaderByNumberer{header: header}
	client := &HTTPClient{blockHeaderByNumberer: provider}

	got, err := client.LatestHeader(requestContext)
	if err != nil {
		t.Fatalf("LatestHeader() error = %v", err)
	}
	if provider.ctx != requestContext || provider.number != nil {
		t.Fatalf("LatestHeader() delegated ctx=%v number=%v, want input context and nil number", provider.ctx, provider.number)
	}
	if got.Number != 456 || got.Hash != header.Hash() || got.ParentHash != header.ParentHash || got.Timestamp != header.Time {
		t.Fatalf("LatestHeader() = %+v, want converted header", got)
	}
}

func TestLatestHeaderUsesBackgroundContextForNilContext(t *testing.T) {
	t.Parallel()

	provider := &fakeBlockHeaderByNumberer{header: &types.Header{Number: big.NewInt(1)}}
	client := &HTTPClient{blockHeaderByNumberer: provider}
	if _, err := client.LatestHeader(nil); err != nil {
		t.Fatalf("LatestHeader() error = %v", err)
	}
	if provider.ctx == nil {
		t.Fatal("LatestHeader() delegated context = nil, want non-nil context")
	}
}

func TestLatestHeaderRejectsInvalidState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		client    *HTTPClient
		wantError string
	}{
		{name: "nil receiver", client: nil, wantError: "failed to get latest evm header: evm_http_client=null"},
		{name: "missing dependency", client: &HTTPClient{}, wantError: "failed to get latest evm header: http_eth_client=null"},
		{name: "nil header", client: &HTTPClient{blockHeaderByNumberer: &fakeBlockHeaderByNumberer{}}, wantError: "failed to get latest evm header: block_header=null"},
		{name: "nil block number", client: &HTTPClient{blockHeaderByNumberer: &fakeBlockHeaderByNumberer{header: &types.Header{}}}, wantError: "failed to get latest evm header: block_number=null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.client.LatestHeader(context.Background())
			if err == nil || err.Error() != tt.wantError {
				t.Fatalf("LatestHeader() error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestLatestHeaderWrapsRPCError(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{blockHeaderByNumberer: &fakeBlockHeaderByNumberer{err: errTestHeaderByNumber}}
	_, err := client.LatestHeader(context.Background())
	if err == nil || !errors.Is(err, errTestHeaderByNumber) {
		t.Fatalf("LatestHeader() error = %v, want wrapped error", err)
	}
}
