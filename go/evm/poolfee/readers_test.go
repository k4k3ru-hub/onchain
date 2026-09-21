package poolfee_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm/poolfee"
	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream"
	v3 "github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3"
	v4 "github.com/k4k3ru-hub/onchain/go/venues/uniswap/v4"
)

type reader struct {
	call func(ethereum.CallMsg, *big.Int) ([]byte, error)
}

// CallContract delegates a pinned read to the injected test implementation.
//
// Version:
//   - 2026-09-22: Added.
func (r reader) CallContract(_ context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	return r.call(msg, block)
}
func word(v uint64) []byte { return new(big.Int).SetUint64(v).FillBytes(make([]byte, 32)) }

// TestV3CreationAndV4DirectionalRates verifies Pool swap fee behavior.
//
// Version:
//   - 2026-09-22: Added.
func TestV3CreationAndV4DirectionalRates(t *testing.T) {
	l := types.Log{Topics: []common.Hash{crypto.Keccak256Hash([]byte("PoolCreated(address,address,uint24,int24,address)")), {}, {}, common.BigToHash(big.NewInt(3000))}, Data: make([]byte, 64)}
	r, err := v3.DecodePoolCreatedFee(l)
	if err != nil || r.Token0ToToken1 != 3000 || r.Token1ToToken0 != 3000 {
		t.Fatal(r, err)
	}
	l.Topics[3] = common.BigToHash(big.NewInt(1_000_000))
	if _, err := v3.DecodePoolCreatedFee(l); err == nil {
		t.Fatal("invalid v3 fee accepted")
	}
	r, err = v4.CombinePoolFees(3000, 1000|500<<12)
	if err != nil || r.Token0ToToken1 != 3997 || r.Token1ToToken0 != 3499 {
		t.Fatal(r, err)
	}
	for _, p := range []uint32{1001, 1001 << 12, 1 << 24} {
		if _, err := v4.CombinePoolFees(3000, p); err == nil {
			t.Fatal("invalid protocol fee accepted")
		}
	}
}

// TestV4PinnedReaderAndUnsupportedHook verifies Pool swap fee behavior.
//
// Version:
//   - 2026-09-22: Added.
func TestV4PinnedReaderAndUnsupportedHook(t *testing.T) {
	calls := 0
	view := common.HexToAddress("0x01")
	id := common.HexToHash("0x02")
	rpc := reader{call: func(msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		calls++
		if block.Uint64() != 42 || *msg.To != view || len(msg.Data) != 36 || common.BytesToHash(msg.Data[4:]) != id {
			t.Fatal("wrong pinned call")
		}
		return append(append(append(word(1), word(0)...), word(1000|500<<12)...), word(3000)...), nil
	}}
	r, err := v4.ReadPoolFees(t.Context(), rpc, view, id, common.Address{}, 3000, big.NewInt(42))
	if err != nil || r.Token0ToToken1 != 3997 || calls != 1 {
		t.Fatal(r, err, calls)
	}
	_, err = v4.ReadPoolFees(t.Context(), rpc, view, id, common.HexToAddress("0x80"), 3000, big.NewInt(42))
	if !errors.Is(err, poolfee.ErrUnsupported) || calls != 1 {
		t.Fatal("hook caused RPC or success", err)
	}
	_, err = v4.ReadPoolFees(t.Context(), rpc, view, id, common.Address{}, 0x800000, big.NewInt(42))
	if !errors.Is(err, poolfee.ErrUnsupported) || calls != 1 {
		t.Fatal("dynamic fee interpreted as rate", err)
	}
}

// TestSlipstreamReadsOnlyFeeAndBindings verifies Pool swap fee behavior.
//
// Version:
//   - 2026-09-22: Added.
func TestSlipstreamReadsOnlyFeeAndBindings(t *testing.T) {
	factory := common.HexToAddress("0x5e7BB104d84c7CB9B682AaC2F3d509f5F406809A")
	module := slipstream.PoolFeeModules(8453, factory)[1]
	pool := common.HexToAddress("0x22")
	sender := common.HexToAddress("0x33")
	calls := 0
	rpc := reader{call: func(msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		calls++
		if block.Uint64() != 42 || msg.From != sender {
			t.Fatal("observation condition changed")
		}
		sig := string(msg.Data)
		switch sig {
		case string(crypto.Keccak256([]byte("factory()"))[:4]):
			if *msg.To != pool && *msg.To != module {
				t.Fatal("wrong binding target")
			}
			return common.LeftPadBytes(factory[:], 32), nil
		case string(crypto.Keccak256([]byte("swapFeeModule()"))[:4]):
			return common.LeftPadBytes(module[:], 32), nil
		case string(crypto.Keccak256([]byte("fee()"))[:4]):
			return word(0), nil
		default:
			t.Fatalf("unexpected history read: %x", msg.Data)
			return nil, nil
		}
	}}
	r, err := slipstream.ReadPoolFees(t.Context(), rpc, 8453, factory, pool, sender, big.NewInt(42))
	if err != nil || calls != 4 || r.Token0ToToken1 != 0 || r.Token1ToToken0 != 0 {
		t.Fatal(r, err, calls)
	}
	module = common.HexToAddress("0x44")
	calls = 0
	_, err = slipstream.ReadPoolFees(t.Context(), rpc, 8453, factory, pool, sender, big.NewInt(42))
	if !errors.Is(err, poolfee.ErrUnsupported) || calls != 2 {
		t.Fatal("unknown module accepted", err, calls)
	}
}
