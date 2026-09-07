package slipstream

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
)

// TestQuoteExactInputSingle verifies quote exact input single in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestQuoteExactInputSingle(t *testing.T) {
	t.Parallel()
	rpc := &factoryTestRPC{response: quoteResponse(24_500_000, 123456, 2, 85_000)}
	quoter := common.HexToAddress("0x0000000000000000000000000000000000000001")
	client, err := NewQuoterClient(QuoterClientParams{RPC: rpc, Quoter: QuoterConfig{Address: quoter}})
	if err != nil {
		t.Fatalf("NewQuoterClient() error = %v", err)
	}
	key := quoteTestPoolKey(t)
	blockNumber := big.NewInt(123)
	result, err := client.QuoteExactInputSingle(context.Background(), QuoteExactInputSingleParams{PoolKey: key, ZeroForOne: true, AmountIn: big.NewInt(10_000_000_000_000_000)}, blockNumber)
	if err != nil {
		t.Fatalf("QuoteExactInputSingle() error = %v", err)
	}
	if result.AmountOut.Uint64() != 24_500_000 || result.SqrtPriceX96After.Uint64() != 123456 || result.InitializedTicksCrossed != 2 || result.GasEstimate.Uint64() != 85_000 {
		t.Fatalf("QuoteExactInputSingle() = %+v", result)
	}
	if rpc.call.To == nil || *rpc.call.To != quoter || rpc.blockNumber != blockNumber || len(rpc.call.Data) != 164 || !bytes.Equal(rpc.call.Data[:4], methodSelector(quoteExactInputSingleSignature)) {
		t.Fatalf("QuoteExactInputSingle() call = %+v block=%v", rpc.call, rpc.blockNumber)
	}
	if got := common.BytesToAddress(rpc.call.Data[4+12 : 4+32]); got != key.Token0.Address() {
		t.Fatalf("QuoteExactInputSingle() tokenIn = %s", got)
	}
}

// TestQuoteExactOutputSingleReversesDirection verifies quote exact output single reverses direction in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestQuoteExactOutputSingleReversesDirection(t *testing.T) {
	t.Parallel()
	rpc := &factoryTestRPC{response: quoteResponse(4_100_000_000_000_000, 654321, 3, 90_000)}
	client, err := NewQuoterClient(QuoterClientParams{RPC: rpc, Quoter: QuoterConfig{Address: common.HexToAddress("0x0000000000000000000000000000000000000001")}})
	if err != nil {
		t.Fatalf("NewQuoterClient() error = %v", err)
	}
	key := quoteTestPoolKey(t)
	result, err := client.QuoteExactOutputSingle(context.Background(), QuoteExactOutputSingleParams{PoolKey: key, ZeroForOne: false, AmountOut: big.NewInt(10_000_000)}, nil)
	if err != nil {
		t.Fatalf("QuoteExactOutputSingle() error = %v", err)
	}
	if result.AmountIn.Uint64() != 4_100_000_000_000_000 || result.InitializedTicksCrossed != 3 {
		t.Fatalf("QuoteExactOutputSingle() = %+v", result)
	}
	if !bytes.Equal(rpc.call.Data[:4], methodSelector(quoteExactOutputSingleSignature)) {
		t.Fatalf("QuoteExactOutputSingle() selector = %x", rpc.call.Data[:4])
	}
	if got := common.BytesToAddress(rpc.call.Data[4+12 : 4+32]); got != key.Token1.Address() {
		t.Fatalf("QuoteExactOutputSingle() tokenIn = %s", got)
	}
}

// TestQuoterClientPropagatesRPCErrors verifies quoter client propagates RPC errors in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestQuoterClientPropagatesRPCErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("rpc unavailable")
	client, err := NewQuoterClient(QuoterClientParams{RPC: &factoryTestRPC{err: wantErr}, Quoter: QuoterConfig{Address: common.HexToAddress("0x0000000000000000000000000000000000000001")}})
	if err != nil {
		t.Fatalf("NewQuoterClient() error = %v", err)
	}
	_, err = client.QuoteExactInputSingle(context.Background(), QuoteExactInputSingleParams{PoolKey: quoteTestPoolKey(t), AmountIn: big.NewInt(1)}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("QuoteExactInputSingle() error = %v", err)
	}
}

// TestQuoterClientRejectsInvalidInputs verifies quoter client rejects invalid inputs in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestQuoterClientRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	if _, err := NewQuoterClient(QuoterClientParams{}); err == nil {
		t.Fatal("NewQuoterClient() error = nil")
	}
	if _, err := NewQuoterClient(QuoterClientParams{RPC: &factoryTestRPC{}}); err == nil {
		t.Fatal("NewQuoterClient() error = nil for empty quoter")
	}
	client, err := NewQuoterClient(QuoterClientParams{RPC: &factoryTestRPC{}, Quoter: QuoterConfig{Address: common.HexToAddress("0x0000000000000000000000000000000000000001")}})
	if err != nil {
		t.Fatalf("NewQuoterClient() error = %v", err)
	}
	if _, err := client.QuoteExactInputSingle(context.Background(), QuoteExactInputSingleParams{PoolKey: quoteTestPoolKey(t)}, nil); err == nil {
		t.Fatal("QuoteExactInputSingle() error = nil")
	}
	if _, err := client.QuoteExactOutputSingle(context.Background(), QuoteExactOutputSingleParams{PoolKey: quoteTestPoolKey(t), AmountOut: big.NewInt(-1)}, nil); err == nil {
		t.Fatal("QuoteExactOutputSingle() error = nil")
	}
}

// TestDecodeSingleQuoteResultRejectsInvalidResponses verifies decode single quote result rejects invalid responses in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestDecodeSingleQuoteResultRejectsInvalidResponses(t *testing.T) {
	t.Parallel()
	for _, response := range [][]byte{nil, make([]byte, 128), quoteResponse(1, 0, 0, 1), quoteResponse(1, 1, 0, 0)} {
		if _, err := decodeSingleQuoteResult(response); err == nil {
			t.Fatalf("decodeSingleQuoteResult(%x) error = nil", response)
		}
	}
}

func quoteTestPoolKey(t *testing.T) protocol.PoolKey {
	t.Helper()
	key, err := protocol.NewPoolKey(
		protocol.NewCurrency(common.HexToAddress("0x4200000000000000000000000000000000000006")),
		protocol.NewCurrency(common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")),
		50,
	)
	if err != nil {
		t.Fatalf("NewPoolKey() error = %v", err)
	}
	return key
}

func quoteResponse(amount, sqrtPrice uint64, ticks uint32, gas uint64) []byte {
	response := make([]byte, 128)
	new(big.Int).SetUint64(amount).FillBytes(response[0:32])
	new(big.Int).SetUint64(sqrtPrice).FillBytes(response[32:64])
	new(big.Int).SetUint64(uint64(ticks)).FillBytes(response[64:96])
	new(big.Int).SetUint64(gas).FillBytes(response[96:128])
	return response
}
