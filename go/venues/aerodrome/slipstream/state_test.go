package slipstream

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestPoolStateClientGetSlot0 verifies pool state client get slot0 in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestPoolStateClientGetSlot0(t *testing.T) {
	t.Parallel()
	rpc := &factoryTestRPC{response: slot0Response(big.NewInt(123456), -42, 1, 2, 3, true)}
	client, err := NewPoolStateClient(PoolStateClientParams{RPC: rpc})
	if err != nil {
		t.Fatalf("NewPoolStateClient() error = %v", err)
	}
	pool := common.HexToAddress("0x0000000000000000000000000000000000000001")
	blockNumber := big.NewInt(123)
	state, err := client.GetSlot0(context.Background(), pool, blockNumber)
	if err != nil {
		t.Fatalf("GetSlot0() error = %v", err)
	}
	if state.SqrtPriceX96.Cmp(big.NewInt(123456)) != 0 || state.Tick != -42 || state.ObservationIndex != 1 || state.ObservationCardinality != 2 || state.ObservationCardinalityNext != 3 || !state.Unlocked {
		t.Fatalf("GetSlot0() = %+v", state)
	}
	if rpc.call.To == nil || *rpc.call.To != pool || rpc.blockNumber != blockNumber || !bytes.Equal(rpc.call.Data, methodSelector(slot0Signature)) {
		t.Fatalf("GetSlot0() call = %+v block=%v", rpc.call, rpc.blockNumber)
	}
}

// TestPoolStateClientGetLiquidity verifies pool state client get liquidity in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestPoolStateClientGetLiquidity(t *testing.T) {
	t.Parallel()
	rpc := &factoryTestRPC{response: uintWord(new(big.Int).Lsh(big.NewInt(1), 100))}
	client, err := NewPoolStateClient(PoolStateClientParams{RPC: rpc})
	if err != nil {
		t.Fatalf("NewPoolStateClient() error = %v", err)
	}
	liquidity, err := client.GetLiquidity(context.Background(), common.HexToAddress("0x0000000000000000000000000000000000000001"), nil)
	if err != nil {
		t.Fatalf("GetLiquidity() error = %v", err)
	}
	if liquidity.BitLen() != 101 || !bytes.Equal(rpc.call.Data, methodSelector(liquiditySignature)) {
		t.Fatalf("GetLiquidity() = %s data=%x", liquidity, rpc.call.Data)
	}
}

// TestPoolStateClientGetTickSpacing verifies pool state client get tick spacing in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestPoolStateClientGetTickSpacing(t *testing.T) {
	t.Parallel()
	rpc := &factoryTestRPC{response: int24Word(50)}
	client, err := NewPoolStateClient(PoolStateClientParams{RPC: rpc})
	if err != nil {
		t.Fatalf("NewPoolStateClient() error = %v", err)
	}
	spacing, err := client.GetTickSpacing(context.Background(), common.HexToAddress("0x0000000000000000000000000000000000000001"), nil)
	if err != nil {
		t.Fatalf("GetTickSpacing() error = %v", err)
	}
	if spacing != 50 || !bytes.Equal(rpc.call.Data, methodSelector(tickSpacingSignature)) {
		t.Fatalf("GetTickSpacing() = %d data=%x", spacing, rpc.call.Data)
	}
}

// TestPoolStateClientPropagatesRPCErrors verifies pool state client propagates RPC errors in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestPoolStateClientPropagatesRPCErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("rpc unavailable")
	client, err := NewPoolStateClient(PoolStateClientParams{RPC: &factoryTestRPC{err: wantErr}})
	if err != nil {
		t.Fatalf("NewPoolStateClient() error = %v", err)
	}
	_, err = client.GetSlot0(context.Background(), common.HexToAddress("0x0000000000000000000000000000000000000001"), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("GetSlot0() error = %v", err)
	}
}

// TestPoolStateClientRejectsInvalidInputs verifies pool state client rejects invalid inputs in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestPoolStateClientRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	if _, err := NewPoolStateClient(PoolStateClientParams{}); err == nil {
		t.Fatal("NewPoolStateClient() error = nil")
	}
	client, err := NewPoolStateClient(PoolStateClientParams{RPC: &factoryTestRPC{}})
	if err != nil {
		t.Fatalf("NewPoolStateClient() error = %v", err)
	}
	if _, err := client.GetSlot0(context.Background(), common.Address{}, nil); err == nil {
		t.Fatal("GetSlot0() error = nil")
	}
	if _, err := client.GetLiquidity(context.Background(), common.HexToAddress("0x0000000000000000000000000000000000000001"), big.NewInt(-1)); err == nil {
		t.Fatal("GetLiquidity() error = nil")
	}
}

// TestStateDecodersRejectInvalidValues verifies state decoders reject invalid values in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestStateDecodersRejectInvalidValues(t *testing.T) {
	t.Parallel()
	if _, err := decodeSlot0(nil); err == nil {
		t.Fatal("decodeSlot0() error = nil")
	}
	invalidSlot0 := slot0Response(big.NewInt(1), 0, 0, 0, 0, false)
	invalidSlot0[160+31] = 2
	if _, err := decodeSlot0(invalidSlot0); err == nil {
		t.Fatal("decodeSlot0() error = nil for invalid bool")
	}
	if _, err := decodeUnsignedWord(uintWord(new(big.Int).Lsh(big.NewInt(1), 128)), 128, "value"); err == nil {
		t.Fatal("decodeUnsignedWord() error = nil")
	}
	if _, err := decodeInt24Word(append([]byte{1}, make([]byte, 31)...), "value"); err == nil {
		t.Fatal("decodeInt24Word() error = nil")
	}
}

func slot0Response(sqrtPrice *big.Int, tick int32, observationIndex, observationCardinality, observationCardinalityNext uint16, unlocked bool) []byte {
	response := make([]byte, 32*6)
	sqrtPrice.FillBytes(response[0:32])
	copy(response[32:64], int24Word(tick))
	new(big.Int).SetUint64(uint64(observationIndex)).FillBytes(response[64:96])
	new(big.Int).SetUint64(uint64(observationCardinality)).FillBytes(response[96:128])
	new(big.Int).SetUint64(uint64(observationCardinalityNext)).FillBytes(response[128:160])
	if unlocked {
		response[191] = 1
	}
	return response
}

func int24Word(value int32) []byte {
	word := make([]byte, 32)
	if value < 0 {
		for i := range word {
			word[i] = 0xff
		}
	}
	raw := uint32(value) & 0xffffff
	word[29] = byte(raw >> 16)
	word[30] = byte(raw >> 8)
	word[31] = byte(raw)
	return word
}

func uintWord(value *big.Int) []byte {
	word := make([]byte, 32)
	value.FillBytes(word)
	return word
}
