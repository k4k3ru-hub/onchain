package clliquidity

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type fakeRPC struct {
	state   State
	pool    Pool
	block   *types.Header
	slots   map[common.Hash][]byte
	calls   int
	failAt  int
	changed bool
	headers int
	wait    bool
	blocks  []uint64
	failure error
}

func newFake(t *testing.T) *fakeRPC {
	s, _ := fixture(t, "robinhood-v4-verified")
	p := Pool{Protocol: V4, Address: common.HexToAddress("0x123"), ID: common.HexToHash("0x456"), Spacing: s.Spacing, V4StorageLayoutVerified: true}
	f := &fakeRPC{state: s, pool: p, block: &types.Header{Number: big.NewInt(100), Time: 1000}, slots: map[common.Hash][]byte{}}
	root := crypto.Keccak256Hash(p.ID[:], intWord(6))
	v := new(big.Int).Lsh(new(big.Int).And(big.NewInt(int64(s.Tick)), big.NewInt(0xffffff)), 160)
	v.Or(v, s.SqrtPriceX96)
	f.slots[root] = word(v)
	f.slots[common.BigToHash(packed(root, 3))] = word(s.ActiveLiquidity)
	for _, tick := range s.Ticks {
		compressed := int64(tick.Index / s.Spacing)
		idx := compressed >> 8
		bit := uint(compressed & 255)
		slot := mapSlot(idx, packed(root, 5))
		b := new(big.Int).SetBytes(f.slots[slot])
		b.SetBit(b, int(bit), 1)
		f.slots[slot] = word(b)
		net := new(big.Int).And(tick.Net, new(big.Int).Sub(power2(128), big.NewInt(1)))
		net.Lsh(net, 128).Or(net, tick.Gross)
		f.slots[mapSlot(int64(tick.Index), packed(root, 4))] = word(net)
	}
	return f
}

// HeaderByNumber supplies a deterministic canonical block to the test reader.
//
// Version:
//   - 2026-09-19: Added.
func (f *fakeRPC) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	f.headers++
	h := types.CopyHeader(f.block)
	if f.changed && f.headers > 1 {
		h.Extra = []byte{1}
	}
	return h, nil
}

// CallContract serves pinned storage or V3 getter fixtures with injected failures.
//
// Version:
//   - 2026-09-19: Added.
func (f *fakeRPC) CallContract(ctx context.Context, m ethereum.CallMsg, n *big.Int) ([]byte, error) {
	f.calls++
	f.blocks = append(f.blocks, n.Uint64())
	if f.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.calls == f.failAt {
		return nil, f.failure
	}
	if string(m.Data[:4]) == string(selector("extsload(bytes32[])")) {
		count := int(new(big.Int).SetBytes(m.Data[36:68]).Int64())
		out := append(uintWord(32), uintWord(count)...)
		for i := 0; i < count; i++ {
			key := common.BytesToHash(m.Data[68+i*32 : 100+i*32])
			value := f.slots[key]
			if value == nil {
				value = make([]byte, 32)
			}
			out = append(out, value...)
		}
		return out, nil
	}
	if string(m.Data[:4]) == string(selector("tryAggregate(bool,(address,bytes)[])")) {
		input, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{{Name: "target", Type: "address"}, {Name: "callData", Type: "bytes"}})
		if err != nil {
			return nil, err
		}
		bt, err := abi.NewType("bool", "", nil)
		if err != nil {
			return nil, err
		}
		v, err := (abi.Arguments{{Type: bt}, {Type: input}}).Unpack(m.Data[4:])
		if err != nil {
			return nil, err
		}
		requests := v[1].([]struct {
			Target   common.Address `json:"target"`
			CallData []byte         `json:"callData"`
		})
		rows := []struct {
			Success    bool
			ReturnData []byte
		}{}
		for _, q := range requests {
			b, err := f.getter(q.CallData)
			if err != nil {
				return nil, err
			}
			rows = append(rows, struct {
				Success    bool
				ReturnData []byte
			}{true, b})
		}
		output, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{{Name: "success", Type: "bool"}, {Name: "returnData", Type: "bytes"}})
		if err != nil {
			return nil, err
		}
		return (abi.Arguments{{Type: output}}).Pack(rows)
	}
	return f.getter(m.Data)
}
func (f *fakeRPC) getter(data []byte) ([]byte, error) {
	sig := string(data[:4])
	s := f.state
	switch sig {
	case string(selector("slot0()")):
		out := append(word(s.SqrtPriceX96), intWord(int64(s.Tick))...)
		return append(out, make([]byte, 160)...), nil
	case string(selector("liquidity()")):
		return word(s.ActiveLiquidity), nil
	case string(selector("tickBitmap(int16)")):
		idx := signedWord(data[4:], 16).Int64()
		b := new(big.Int)
		for _, t := range s.Ticks {
			compressed := int64(t.Index / s.Spacing)
			if compressed>>8 == idx {
				b.SetBit(b, int(compressed&255), 1)
			}
		}
		return word(b), nil
	case string(selector("ticks(int24)")):
		idx := signedWord(data[4:], 24).Int64()
		for _, t := range s.Ticks {
			if int64(t.Index) == idx {
				return append(append(word(t.Gross), word(t.Net)...), make([]byte, 192)...), nil
			}
		}
	}
	return nil, errors.New("unknown getter")
}
func limits() Limits {
	return Limits{Timeout: time.Second, ChunkSize: 16, MaxReads: 500, MaxCalls: 100, MaxTicks: 100, MaxResponseBytes: 100000}
}
func TestCapturePinnedStateAndMulticall(t *testing.T) {
	for _, mode := range []string{"v4", "v3", "multicall"} {
		t.Run(mode, func(t *testing.T) {
			f := newFake(t)
			if mode != "v4" {
				f.pool.Protocol = V3
			}
			if mode == "multicall" {
				f.pool.Multicall = common.HexToAddress("0x987")
			}
			r, err := NewReader(f, limits())
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Capture(context.Background(), f.pool)
			if err != nil {
				t.Fatal(err)
			}
			want, err := Calculate(f.state)
			if err != nil {
				t.Fatal(err)
			}
			if got.Amounts.Token0.Cmp(want.Token0) != 0 || got.Amounts.Token1.Cmp(want.Token1) != 0 {
				t.Fatal("amount mismatch")
			}
			for _, n := range f.blocks {
				if n != 100 {
					t.Fatal("mixed block")
				}
			}
			if !got.State.Complete {
				t.Fatal("incomplete")
			}
		})
	}
}
func TestCaptureFailureIsAtomic(t *testing.T) {
	underlying := errors.New("archive unavailable")
	for _, mode := range []string{"rpc", "reorg", "reads", "calls", "ticks", "bytes", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			f := newFake(t)
			l := limits()
			var want error
			switch mode {
			case "rpc":
				f.failAt = 3
				f.failure = underlying
				want = underlying
			case "reorg":
				f.changed = true
			case "reads":
				l.MaxReads = 2
				want = ErrBudget
			case "calls":
				l.MaxCalls = 2
				want = ErrBudget
			case "ticks":
				l.MaxTicks = 1
				want = ErrBudget
			case "bytes":
				l.MaxResponseBytes = 32
				want = ErrBudget
			case "deadline":
				l.Timeout = time.Millisecond
				f.wait = true
				want = context.DeadlineExceeded
			}
			r, err := NewReader(f, l)
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Capture(context.Background(), f.pool)
			if err == nil || got.Amounts.Token0 != nil || got.State.Complete {
				t.Fatalf("published partial: %v %v", got, err)
			}
			if want != nil && !errors.Is(err, want) {
				t.Fatalf("error chain: %v", err)
			}
		})
	}
}
