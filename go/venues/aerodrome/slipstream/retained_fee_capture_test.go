package slipstream

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type feeCaptureFake struct {
	*oracleCaptureFake
	module, factory, pool common.Address
	malformed             bool
	failure               error
	originChecked         bool
}

// CallContract serves fee settings only at the requested baseline block.
//
// Version:
//   - 2026-09-09: Added.
func (f *feeCaptureFake) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if block == nil || block.Uint64() != 100 || msg.From != (common.Address{}) {
		f.t.Fatal("wrong block or origin")
	}
	if f.failure != nil {
		return nil, f.failure
	}
	sig := string(msg.Data[:4])
	selector := func(value string) string { return string(crypto.Keccak256([]byte(value))[:4]) }
	word := func(value uint64) []byte { return new(big.Int).SetUint64(value).FillBytes(make([]byte, 32)) }
	switch sig {
	case selector("swapFeeModule()"):
		if *msg.To != f.factory {
			f.t.Fatal("wrong factory")
		}
		return common.LeftPadBytes(f.module[:], 32), nil
	case selector("factory()"):
		if *msg.To != f.module {
			f.t.Fatal("wrong module")
		}
		return common.LeftPadBytes(f.factory[:], 32), nil
	case selector("dynamicFeeConfig(address)"):
		if *msg.To != f.module || common.BytesToAddress(msg.Data[4:]) != f.pool {
			f.t.Fatal("wrong pool config")
		}
		if f.malformed {
			return make([]byte, 96), nil
		}
		data := make([]byte, 160)
		copy(data[:32], word(3000))
		return data, nil
	case selector("tickSpacingToFee(int24)"):
		if *msg.To != f.factory || new(big.Int).SetBytes(msg.Data[4:]).Uint64() != 60 {
			f.t.Fatal("wrong spacing")
		}
		return word(3000), nil
	case selector("defaultScalingFactor()"):
		return word(1000000), nil
	case selector("defaultFeeCap()"):
		return word(50000), nil
	case selector("secondsAgo()"):
		return word(600), nil
	case selector("discounted(address)"):
		if new(big.Int).SetBytes(msg.Data[4:]).Sign() != 0 {
			f.t.Fatal("trade origin leaked into quote")
		}
		f.originChecked = true
		return word(0), nil
	default:
		return f.oracleCaptureFake.CallContract(ctx, msg, block)
	}
}

// TestCaptureRetainedFeeState verifies local parity, pinned reads and failure isolation.
//
// Version:
//   - 2026-09-09: Added.
func TestCaptureRetainedFeeState(t *testing.T) {
	f := &feeCaptureFake{oracleCaptureFake: &oracleCaptureFake{stateFake: &stateFake{t: t}}, module: common.HexToAddress("0x10"), factory: common.HexToAddress("0x20"), pool: common.HexToAddress("0x30")}
	c := &StateCache{rpc: f, factory: f.factory, pool: f.pool}
	s := &poolSnapshot{header: evm.BlockHeader{Number: 100, Hash: common.HexToHash("01"), Timestamp: 100}, price: power2(96), spacing: 60, fee: 3000}
	got, err := c.captureRetainedFeeState(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if got.module != f.module || got.config.base != 3000 || len(got.oracle.slots) != 129 || !f.originChecked {
		t.Fatal("incomplete baseline")
	}
	for _, mode := range []string{"fee mismatch", "reorg", "unknown ABI", "rpc failure"} {
		t.Run(mode, func(t *testing.T) {
			s.fee = 3000
			f.reorg, f.malformed, f.failure = false, false, nil
			sentinel := errors.New("rpc failed")
			switch mode {
			case "fee mismatch":
				s.fee = 3001
			case "reorg":
				f.reorg = true
			case "unknown ABI":
				f.malformed = true
			case "rpc failure":
				f.failure = sentinel
			}
			state, err := c.captureRetainedFeeState(context.Background(), s)
			if err == nil || state.module != (common.Address{}) || len(state.oracle.slots) != 0 {
				t.Fatalf("partial capture accepted: %v", err)
			}
			if mode == "rpc failure" && !errors.Is(err, sentinel) {
				t.Fatalf("lost underlying error: %v", err)
			}
		})
	}
}

// TestDecodeRetainedFeeConfig rejects noncanonical and out-of-range settings.
//
// Version:
//   - 2026-09-09: Added.
func TestDecodeRetainedFeeConfig(t *testing.T) {
	for index, value := range []uint64{30001, 50001, 1000000000000000001, 2, 50001} {
		data := make([]byte, 160)
		new(big.Int).SetUint64(value).FillBytes(data[index*32 : (index+1)*32])
		if _, err := decodeRetainedFeeConfig(data); err == nil {
			t.Fatalf("accepted field %d", index)
		}
	}
}
