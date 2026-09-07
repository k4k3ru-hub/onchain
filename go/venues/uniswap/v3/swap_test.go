package v3

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func testSwapLog(t *testing.T, pool common.Address) types.Log {
	t.Helper()
	args := abi.Arguments{{Type: mustABIType(t, "int256")}, {Type: mustABIType(t, "int256")}, {Type: mustABIType(t, "uint160")}, {Type: mustABIType(t, "uint128")}, {Type: mustABIType(t, "int24")}}
	data, err := args.Pack(big.NewInt(100), big.NewInt(-90), big.NewInt(123), big.NewInt(456), big.NewInt(-7))
	if err != nil {
		t.Fatal(err)
	}
	return types.Log{Address: pool, Topics: []common.Hash{swapEventSignatureHash(), common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x03").Bytes(), 32)), common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x04").Bytes(), 32))}, Data: data, BlockNumber: 10, Index: 2}
}

func TestDecodeSwapLog(t *testing.T) {
	t.Parallel()
	swap, err := DecodeSwapLog(testSwapLog(t, common.HexToAddress("0x05")))
	if err != nil {
		t.Fatalf("DecodeSwapLog() error = %v", err)
	}
	if swap.Amount0.Cmp(big.NewInt(100)) != 0 || swap.Amount1.Cmp(big.NewInt(-90)) != 0 || swap.Tick != -7 || swap.Sender != common.HexToAddress("0x03") {
		t.Fatalf("DecodeSwapLog() = %+v", swap)
	}
}

func TestFilterSwaps(t *testing.T) {
	t.Parallel()
	config := testFactoryConfig(t)
	pool, err := config.PoolKeys[0].Address(config.Address, config.InitCodeHash)
	if err != nil {
		t.Fatal(err)
	}
	rpc := &testHTTPRPC{logs: []types.Log{testSwapLog(t, pool)}}
	client, err := NewHTTPClient(HTTPClientParams{RPC: rpc, Factory: config})
	if err != nil {
		t.Fatal(err)
	}
	swaps, err := client.FilterSwaps(context.Background(), big.NewInt(1), big.NewInt(2))
	if err != nil {
		t.Fatalf("FilterSwaps() error = %v", err)
	}
	if len(swaps) != 1 || swaps[0].PoolKey.Fee != config.PoolKeys[0].Fee {
		t.Fatalf("FilterSwaps() = %+v", swaps)
	}
	if len(rpc.query.Addresses) != 1 || rpc.query.Addresses[0] != pool {
		t.Fatalf("query = %+v", rpc.query)
	}
}
