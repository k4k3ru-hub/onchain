package evm

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type fakeCodeReader struct {
	code    []byte
	err     error
	ctx     context.Context
	address common.Address
	hash    common.Hash
}

// CodeAtHash records a fixed-hash code request for delegation tests.
//
// Version:
//   - 2026-09-22: Added.
func (f *fakeCodeReader) CodeAtHash(ctx context.Context, address common.Address, hash common.Hash) ([]byte, error) {
	f.ctx, f.address, f.hash = ctx, address, hash
	return f.code, f.err
}

// TestHTTPClientCodeAtHash verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestHTTPClientCodeAtHash(t *testing.T) {
	reader := &fakeCodeReader{code: []byte{1, 2, 3}}
	client := &HTTPClient{codeReader: reader}
	address, hash := common.HexToAddress("0x1234"), common.HexToHash("0x5678")
	code, err := client.CodeAtHash(nil, address, hash)
	if err != nil {
		t.Fatal(err)
	}
	if reader.ctx == nil || reader.address != address || reader.hash != hash || len(code) != 3 {
		t.Fatal("request was not delegated")
	}
	code[0] = 9
	if reader.code[0] != 1 {
		t.Fatal("code aliases reader storage")
	}
	failure := errors.New("rpc failure")
	reader.err = failure
	if _, err := client.CodeAtHash(context.Background(), address, hash); !errors.Is(err, failure) {
		t.Fatal("error chain lost")
	}
	for _, tc := range []struct {
		client  *HTTPClient
		address common.Address
		hash    common.Hash
	}{
		{nil, address, hash}, {&HTTPClient{}, address, hash}, {client, common.Address{}, hash}, {client, address, common.Hash{}},
	} {
		if _, err := tc.client.CodeAtHash(nil, tc.address, tc.hash); err == nil {
			t.Fatal("invalid dependency or block accepted")
		}
	}
}

// TestComposeHTTPClientCodeReader verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestComposeHTTPClientCodeReader(t *testing.T) {
	transport := new(ethclient.Client)
	client := composeHTTPClient(HTTPConfig{}, transport)
	if client.codeReader != transport {
		t.Fatal("code reader missing from composition root")
	}
}
