package v3

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/deployment"
	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
)

type testHTTPRPC struct {
	call     ethereum.CallMsg
	block    *big.Int
	response []byte
	err      error
	query    ethereum.FilterQuery
	logs     []types.Log
}

func (r *testHTTPRPC) CallContract(_ context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	r.call = call
	r.block = block
	return r.response, r.err
}
func (r *testHTTPRPC) FilterLogs(_ context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	r.query = query
	return r.logs, r.err
}

type testWSRPC struct{}

func (*testWSRPC) SubscribeFilterLogs(context.Context, ethereum.FilterQuery, chan<- types.Log) (ethereum.Subscription, error) {
	return nil, nil
}

func testPoolKey() protocol.PoolKey {
	return protocol.PoolKey{Token0: protocol.NewCurrency(common.HexToAddress("0x01")), Token1: protocol.NewCurrency(common.HexToAddress("0x02")), Fee: 3000}
}
func testFactoryConfig(t *testing.T) FactoryConfig {
	t.Helper()
	d, err := deployment.ByChainID(1)
	if err != nil {
		t.Fatal(err)
	}
	return FactoryConfig{Address: d.Factory, QuoterV2: d.QuoterV2, InitCodeHash: d.InitCodeHash, PoolKeys: []protocol.PoolKey{testPoolKey()}}
}

func TestNewClient(t *testing.T) {
	t.Parallel()
	client, err := NewClient(ClientParams{HTTPRPCClient: &testHTTPRPC{}, WSRPCClient: &testWSRPC{}, Factory: testFactoryConfig(t)})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.HTTP == nil || client.WS == nil {
		t.Fatalf("NewClient() = %+v", client)
	}
}

func TestNewHTTPAndWSClientsAreIndependent(t *testing.T) {
	t.Parallel()
	if _, err := NewHTTPClient(HTTPClientParams{RPC: &testHTTPRPC{}, Factory: testFactoryConfig(t)}); err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}
	config := testFactoryConfig(t)
	config.QuoterV2 = common.Address{}
	if _, err := NewWSClient(WSClientParams{RPC: &testWSRPC{}, Factory: config}); err != nil {
		t.Fatalf("NewWSClient() error = %v", err)
	}
}
