package slipstream

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
)

type factoryTestRPC struct {
	response    []byte
	err         error
	call        ethereum.CallMsg
	blockNumber *big.Int
}

// CallContract records the request and returns the injected response.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func (r *factoryTestRPC) CallContract(_ context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	r.call = call
	r.blockNumber = blockNumber
	return r.response, r.err
}

// TestFactoryClientResolvePool verifies factory client resolve pool in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestFactoryClientResolvePool(t *testing.T) {
	t.Parallel()
	factory := common.HexToAddress("0x0000000000000000000000000000000000000001")
	pool := common.HexToAddress("0x0000000000000000000000000000000000000002")
	rpc := &factoryTestRPC{response: addressResponse(pool)}
	client, err := NewFactoryClient(FactoryClientParams{RPC: rpc, Factory: FactoryConfig{Address: factory}})
	if err != nil {
		t.Fatalf("NewFactoryClient() error = %v", err)
	}
	key, err := protocol.NewPoolKey(
		protocol.NewCurrency(common.HexToAddress("0x4200000000000000000000000000000000000006")),
		protocol.NewCurrency(common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")),
		50,
	)
	if err != nil {
		t.Fatalf("NewPoolKey() error = %v", err)
	}
	blockNumber := big.NewInt(123)
	got, err := client.ResolvePool(context.Background(), key, blockNumber)
	if err != nil {
		t.Fatalf("ResolvePool() error = %v", err)
	}
	if got != pool || rpc.call.To == nil || *rpc.call.To != factory || rpc.blockNumber != blockNumber {
		t.Fatalf("ResolvePool() = %s call=%+v block=%v", got, rpc.call, rpc.blockNumber)
	}
	if !bytes.Equal(rpc.call.Data[:4], methodSelector(getPoolSignature)) || len(rpc.call.Data) != 100 {
		t.Fatalf("ResolvePool() data = %x", rpc.call.Data)
	}
}

// TestFactoryClientGetSwapFee verifies factory client get swap fee in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestFactoryClientGetSwapFee(t *testing.T) {
	t.Parallel()
	factory := common.HexToAddress("0x0000000000000000000000000000000000000001")
	pool := common.HexToAddress("0x0000000000000000000000000000000000000002")
	rpc := &factoryTestRPC{response: uintResponse(500)}
	client, err := NewFactoryClient(FactoryClientParams{RPC: rpc, Factory: FactoryConfig{Address: factory}})
	if err != nil {
		t.Fatalf("NewFactoryClient() error = %v", err)
	}
	fee, err := client.GetSwapFee(context.Background(), pool, nil)
	if err != nil {
		t.Fatalf("GetSwapFee() error = %v", err)
	}
	if fee != 500 || !bytes.Equal(rpc.call.Data[:4], methodSelector(getSwapFeeSignature)) || len(rpc.call.Data) != 36 {
		t.Fatalf("GetSwapFee() = %d data=%x", fee, rpc.call.Data)
	}
}

// TestFactoryClientPropagatesRPCErrors verifies factory client propagates RPC errors in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestFactoryClientPropagatesRPCErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("rpc unavailable")
	rpc := &factoryTestRPC{err: wantErr}
	client, err := NewFactoryClient(FactoryClientParams{RPC: rpc, Factory: FactoryConfig{Address: common.HexToAddress("0x0000000000000000000000000000000000000001")}})
	if err != nil {
		t.Fatalf("NewFactoryClient() error = %v", err)
	}
	key, err := protocol.NewPoolKey(
		protocol.NewCurrency(common.HexToAddress("0x0000000000000000000000000000000000000002")),
		protocol.NewCurrency(common.HexToAddress("0x0000000000000000000000000000000000000003")),
		1,
	)
	if err != nil {
		t.Fatalf("NewPoolKey() error = %v", err)
	}
	if _, err := client.ResolvePool(context.Background(), key, nil); !errors.Is(err, wantErr) {
		t.Fatalf("ResolvePool() error = %v", err)
	}
	if _, err := client.GetSwapFee(context.Background(), common.HexToAddress("0x0000000000000000000000000000000000000002"), nil); !errors.Is(err, wantErr) {
		t.Fatalf("GetSwapFee() error = %v", err)
	}
}

// TestFactoryClientRejectsInvalidInputs verifies factory client rejects invalid inputs in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestFactoryClientRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	if _, err := NewFactoryClient(FactoryClientParams{}); err == nil {
		t.Fatal("NewFactoryClient() error = nil")
	}
	if _, err := NewFactoryClient(FactoryClientParams{RPC: &factoryTestRPC{}}); err == nil {
		t.Fatal("NewFactoryClient() error = nil for empty factory")
	}
	client, err := NewFactoryClient(FactoryClientParams{RPC: &factoryTestRPC{}, Factory: FactoryConfig{Address: common.HexToAddress("0x0000000000000000000000000000000000000001")}})
	if err != nil {
		t.Fatalf("NewFactoryClient() error = %v", err)
	}
	if _, err := client.ResolvePool(context.Background(), protocol.PoolKey{}, nil); err == nil {
		t.Fatal("ResolvePool() error = nil")
	}
	if _, err := client.GetSwapFee(context.Background(), common.Address{}, nil); err == nil {
		t.Fatal("GetSwapFee() error = nil")
	}
}

func addressResponse(address common.Address) []byte {
	response := make([]byte, 32)
	copy(response[12:], address.Bytes())
	return response
}

func uintResponse(value uint64) []byte {
	response := make([]byte, 32)
	new(big.Int).SetUint64(value).FillBytes(response)
	return response
}
