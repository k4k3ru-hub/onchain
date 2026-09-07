package v3

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func mustABIType(t *testing.T, name string) abi.Type {
	t.Helper()
	value, err := abi.NewType(name, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestGetSlot0(t *testing.T) {
	t.Parallel()
	config := testFactoryConfig(t)
	pool, err := config.PoolKeys[0].Address(config.Address, config.InitCodeHash)
	if err != nil {
		t.Fatal(err)
	}
	args := abi.Arguments{{Type: mustABIType(t, "uint160")}, {Type: mustABIType(t, "int24")}, {Type: mustABIType(t, "uint16")}, {Type: mustABIType(t, "uint16")}, {Type: mustABIType(t, "uint16")}, {Type: mustABIType(t, "uint8")}, {Type: mustABIType(t, "bool")}}
	response, err := args.Pack(big.NewInt(123), big.NewInt(-5), uint16(1), uint16(2), uint16(3), uint8(4), true)
	if err != nil {
		t.Fatal(err)
	}
	rpc := &testHTTPRPC{response: response}
	client, err := NewHTTPClient(HTTPClientParams{RPC: rpc, Factory: config})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetSlot0(context.Background(), pool, big.NewInt(100))
	if err != nil {
		t.Fatalf("GetSlot0() error = %v", err)
	}
	if got.SqrtPriceX96.Cmp(big.NewInt(123)) != 0 || got.Tick != -5 || got.ObservationCardinalityNext != 3 || !got.Unlocked {
		t.Fatalf("GetSlot0() = %+v", got)
	}
	if rpc.call.To == nil || *rpc.call.To != pool || string(rpc.call.Data[:4]) != string(crypto.Keccak256([]byte(slot0Signature))[:4]) {
		t.Fatalf("CallContract() = %+v", rpc.call)
	}
}

func TestQuoteExactInputSingle(t *testing.T) {
	t.Parallel()
	args := abi.Arguments{{Type: mustABIType(t, "uint256")}, {Type: mustABIType(t, "uint160")}, {Type: mustABIType(t, "uint32")}, {Type: mustABIType(t, "uint256")}}
	response, err := args.Pack(big.NewInt(99), big.NewInt(123), uint32(7), big.NewInt(456))
	if err != nil {
		t.Fatal(err)
	}
	rpc := &testHTTPRPC{response: response}
	config := testFactoryConfig(t)
	client, err := NewHTTPClient(HTTPClientParams{RPC: rpc, Factory: config})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.QuoteExactInputSingle(context.Background(), QuoteExactInputSingleParams{PoolKey: config.PoolKeys[0], ZeroForOne: true, AmountIn: big.NewInt(100)}, nil)
	if err != nil {
		t.Fatalf("QuoteExactInputSingle() error = %v", err)
	}
	if result.AmountOut.Cmp(big.NewInt(99)) != 0 || result.InitializedTicksCrossed != 7 || rpc.call.To == nil || *rpc.call.To != config.QuoterV2 {
		t.Fatalf("result = %+v call = %+v", result, rpc.call)
	}
	if got, want := common.Bytes2Hex(rpc.call.Data[:4]), common.Bytes2Hex(crypto.Keccak256([]byte(quoteExactInputSingleSignature))[:4]); got != want {
		t.Fatalf("selector = %s, want %s", got, want)
	}
}

func TestQuoteRejectsInvalidAmount(t *testing.T) {
	t.Parallel()
	client, err := NewHTTPClient(HTTPClientParams{RPC: &testHTTPRPC{}, Factory: testFactoryConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.QuoteExactOutputSingle(context.Background(), QuoteExactOutputSingleParams{PoolKey: testPoolKey()}, nil); err == nil {
		t.Fatal("QuoteExactOutputSingle() error = nil")
	}
}
