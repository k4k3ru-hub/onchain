package clliquidity

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type RPC interface {
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
}

type Protocol uint8

const (
	V3 Protocol = iota + 1
	Slipstream
	V4
)

type Pool struct {
	Protocol Protocol
	// Address is the pool contract for V3/Slipstream, or the V4 PoolManager.
	Address common.Address
	ID      common.Hash
	Spacing int32
	// Multicall must be a configured, trusted deployment. Zero uses individual getters.
	Multicall common.Address
	// V4StorageLayoutVerified permits the canonical V4-core storage layout.
	// Deployment verification belongs to the composition boundary, not user input.
	V4StorageLayoutVerified bool
}

type Limits struct {
	Timeout          time.Duration
	ChunkSize        int
	MaxReads         int
	MaxCalls         int
	MaxTicks         int
	MaxResponseBytes int
}

type Reader struct {
	rpc    RPC
	limits Limits
}

type Snapshot struct {
	BlockNumber                 uint64
	BlockHash                   common.Hash
	BlockTime                   time.Time
	CapturedAt                  time.Time
	State                       State
	Amounts                     Amounts
	Reads, Calls, ResponseBytes int
}

var ErrBudget = errors.New("lp state budget exceeded")

// NewReader composes a bounded principal reader without making RPC requests.
// Limits are explicit so a library does not prescribe provider-specific budgets.
//
// Version:
//   - 2026-09-19: Added.
func NewReader(rpc RPC, limits Limits) (*Reader, error) {
	if rpc == nil {
		return nil, fmt.Errorf("failed to create lp reader: rpc=null")
	}
	if limits.Timeout <= 0 || limits.ChunkSize < 1 || limits.ChunkSize > 256 || limits.MaxReads < 2 || limits.MaxCalls < 2 || limits.MaxTicks < 1 || limits.MaxResponseBytes < 32 {
		return nil, fmt.Errorf("failed to create lp reader: limits=out_of_range")
	}
	return &Reader{rpc, limits}, nil
}

type capture struct {
	reader              *Reader
	ctx                 context.Context
	block               *big.Int
	reads, calls, bytes int
}

// Capture reads a complete principal snapshot at one recent canonical block.
// The deadline includes scheduling and all chunks. Any failure returns a zero
// snapshot; no retries, latest-block substitutions or partial results are made.
//
// Version:
//   - 2026-09-19: Added.
func (r *Reader) Capture(ctx context.Context, p Pool) (Snapshot, error) {
	if r == nil || ctx == nil {
		return Snapshot{}, fmt.Errorf("failed to capture lp state: dependency=null")
	}
	if p.Address == (common.Address{}) || p.Spacing < 1 || p.Spacing > 32767 || p.Protocol < V3 || p.Protocol > V4 {
		return Snapshot{}, fmt.Errorf("failed to capture lp state: pool=invalid")
	}
	if p.Protocol == V4 && (!p.V4StorageLayoutVerified || p.ID == (common.Hash{})) {
		return Snapshot{}, fmt.Errorf("failed to capture lp state: v4_layout=unverified")
	}
	ctx, cancel := context.WithTimeout(ctx, r.limits.Timeout)
	defer cancel()
	c := &capture{reader: r, ctx: ctx}
	h, err := c.header(nil)
	if err != nil {
		return Snapshot{}, err
	}
	c.block = new(big.Int).Set(h.Number)
	s, err := c.readState(p)
	if err != nil {
		return Snapshot{}, fmt.Errorf("failed to capture lp state: %w", err)
	}
	amounts, err := Calculate(s)
	if err != nil {
		return Snapshot{}, fmt.Errorf("failed to capture lp state: %w", err)
	}
	after, err := c.header(c.block)
	if err != nil {
		return Snapshot{}, err
	}
	if after.Number.Cmp(h.Number) != 0 || after.Hash() != h.Hash() {
		return Snapshot{}, fmt.Errorf("failed to capture lp state: block=mismatch")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("failed to capture lp state: %w", err)
	}
	return Snapshot{BlockNumber: h.Number.Uint64(), BlockHash: h.Hash(), BlockTime: time.Unix(int64(h.Time), 0).UTC(), CapturedAt: time.Now().UTC(), State: s, Amounts: amounts, Reads: c.reads, Calls: c.calls, ResponseBytes: c.bytes}, nil
}

func (c *capture) budget(reads int) error {
	if err := c.ctx.Err(); err != nil {
		return fmt.Errorf("failed to read lp state: %w", err)
	}
	if reads > c.reader.limits.MaxReads-c.reads || c.calls >= c.reader.limits.MaxCalls {
		return fmt.Errorf("failed to read lp state: %w", ErrBudget)
	}
	c.reads += reads
	c.calls++
	return nil
}
func (c *capture) header(n *big.Int) (*types.Header, error) {
	if err := c.budget(0); err != nil {
		return nil, err
	}
	h, err := c.reader.rpc.HeaderByNumber(c.ctx, n)
	if err != nil {
		return nil, fmt.Errorf("failed to read lp header: %w", err)
	}
	if h == nil || h.Number == nil || !h.Number.IsUint64() || h.Time > uint64(1<<63-1) {
		return nil, fmt.Errorf("failed to read lp header: header=invalid")
	}
	return h, nil
}
func (c *capture) call(target common.Address, data []byte, reads int) ([]byte, error) {
	if err := c.budget(reads); err != nil {
		return nil, err
	}
	b, err := c.reader.rpc.CallContract(c.ctx, ethereum.CallMsg{To: &target, Data: data}, new(big.Int).Set(c.block))
	if err != nil {
		return nil, fmt.Errorf("failed to read lp contract: %w", err)
	}
	if len(b) > c.reader.limits.MaxResponseBytes-c.bytes {
		return nil, fmt.Errorf("failed to read lp contract: %w", ErrBudget)
	}
	c.bytes += len(b)
	if err := c.ctx.Err(); err != nil {
		return nil, fmt.Errorf("failed to read lp contract: %w", err)
	}
	return b, nil
}
func selector(s string) []byte { return crypto.Keccak256([]byte(s))[:4] }
func word(n *big.Int) []byte {
	v := new(big.Int).Set(n)
	if v.Sign() < 0 {
		v.Add(v, power2(256))
	}
	return v.FillBytes(make([]byte, 32))
}
func intWord(n int64) []byte { return word(big.NewInt(n)) }
func uintWord(n int) []byte  { return intWord(int64(n)) }
func signedWord(b []byte, bits uint) *big.Int {
	n := new(big.Int).SetBytes(b)
	n.And(n, new(big.Int).Sub(power2(bits), big.NewInt(1)))
	if n.Bit(int(bits-1)) == 1 {
		n.Sub(n, power2(bits))
	}
	return n
}
func query(sig string, arg *big.Int) []byte {
	out := append([]byte(nil), selector(sig)...)
	if arg != nil {
		out = append(out, word(arg)...)
	}
	return out
}
func packed(s common.Hash, offset int64) *big.Int {
	return new(big.Int).Add(new(big.Int).SetBytes(s[:]), big.NewInt(offset))
}
func mapSlot(key int64, base *big.Int) common.Hash {
	return crypto.Keccak256Hash(intWord(key), word(base))
}

func (c *capture) readState(p Pool) (State, error) {
	s := State{Spacing: p.Spacing}
	var root common.Hash
	var head [][]byte
	var err error
	if p.Protocol == V4 {
		root = crypto.Keccak256Hash(p.ID[:], intWord(6))
		head, err = c.storage(p.Address, []common.Hash{root, common.BigToHash(packed(root, 3))})
	} else {
		head, err = c.getters(p, [][]byte{query("slot0()", nil), query("liquidity()", nil)})
	}
	if err != nil {
		return s, err
	}
	if len(head) != 2 || len(head[1]) != 32 {
		return s, fmt.Errorf("failed to decode lp state: header=invalid")
	}
	if p.Protocol == V4 {
		if len(head[0]) != 32 {
			return s, fmt.Errorf("failed to decode lp state: slot0=invalid")
		}
		s.SqrtPriceX96 = new(big.Int).SetBytes(head[0][12:])
		s.Tick = int32(signedWord(head[0][9:12], 24).Int64())
	} else {
		expected := 224
		if p.Protocol == Slipstream {
			expected = 192
		}
		if len(head[0]) != expected {
			return s, fmt.Errorf("failed to decode lp state: slot0=invalid")
		}
		s.SqrtPriceX96 = new(big.Int).SetBytes(head[0][:32])
		s.Tick = int32(signedWord(head[0][32:64], 24).Int64())
	}
	s.ActiveLiquidity = new(big.Int).SetBytes(head[1])
	n := int64(887272 / p.Spacing)
	lower := (-n - 255) / 256
	upper := n / 256
	count := int(upper - lower + 1)
	if count > c.reader.limits.MaxReads-c.reads {
		return s, fmt.Errorf("failed to read lp bitmap: %w", ErrBudget)
	}
	calls := make([][]byte, 0, count)
	slots := make([]common.Hash, 0, count)
	for i := lower; i <= upper; i++ {
		if p.Protocol == V4 {
			slots = append(slots, mapSlot(i, packed(root, 5)))
		} else {
			calls = append(calls, query("tickBitmap(int16)", big.NewInt(i)))
		}
	}
	var maps [][]byte
	if p.Protocol == V4 {
		maps, err = c.storage(p.Address, slots)
	} else {
		maps, err = c.getters(p, calls)
	}
	if err != nil {
		return s, err
	}
	indexes := []int32{}
	for i, b := range maps {
		if len(b) != 32 {
			return s, fmt.Errorf("failed to decode lp bitmap: word=invalid")
		}
		bits := new(big.Int).SetBytes(b)
		for bit := 0; bit < 256; bit++ {
			if bits.Bit(bit) == 0 {
				continue
			}
			idx := ((lower+int64(i))*256 + int64(bit)) * int64(p.Spacing)
			if idx < -887272 || idx > 887272 {
				return s, fmt.Errorf("failed to decode lp bitmap: tick=out_of_range")
			}
			indexes = append(indexes, int32(idx))
			if len(indexes) > c.reader.limits.MaxTicks {
				return s, fmt.Errorf("failed to read lp ticks: %w", ErrBudget)
			}
		}
	}
	if len(indexes) > c.reader.limits.MaxReads-c.reads {
		return s, fmt.Errorf("failed to read lp ticks: %w", ErrBudget)
	}
	calls = nil
	slots = nil
	for _, idx := range indexes {
		if p.Protocol == V4 {
			slots = append(slots, mapSlot(int64(idx), packed(root, 4)))
		} else {
			calls = append(calls, query("ticks(int24)", big.NewInt(int64(idx))))
		}
	}
	var ticks [][]byte
	if p.Protocol == V4 {
		ticks, err = c.storage(p.Address, slots)
	} else {
		ticks, err = c.getters(p, calls)
	}
	if err != nil {
		return s, err
	}
	for i, b := range ticks {
		var gross, net *big.Int
		if p.Protocol == V4 {
			if len(b) != 32 {
				return s, fmt.Errorf("failed to decode lp tick: word=invalid")
			}
			gross = new(big.Int).SetBytes(b[16:])
			net = signedWord(b[:16], 128)
		} else {
			if len(b) < 64 || len(b)%32 != 0 {
				return s, fmt.Errorf("failed to decode lp tick: words=invalid")
			}
			gross = new(big.Int).SetBytes(b[:32])
			net = signedWord(b[32:64], 128)
		}
		s.Ticks = append(s.Ticks, Tick{indexes[i], gross, net})
	}
	s.Complete = true
	return s, nil
}

func (c *capture) storage(target common.Address, slots []common.Hash) ([][]byte, error) {
	out := make([][]byte, 0, len(slots))
	for start := 0; start < len(slots); start += c.reader.limits.ChunkSize {
		part := slots[start:min(start+c.reader.limits.ChunkSize, len(slots))]
		data := append(append(selector("extsload(bytes32[])"), uintWord(32)...), uintWord(len(part))...)
		for _, slot := range part {
			data = append(data, slot[:]...)
		}
		b, err := c.call(target, data, len(part))
		if err != nil {
			return nil, err
		}
		if len(b) != 64+len(part)*32 || new(big.Int).SetBytes(b[:32]).Cmp(big.NewInt(32)) != 0 || new(big.Int).SetBytes(b[32:64]).Cmp(big.NewInt(int64(len(part)))) != 0 {
			return nil, fmt.Errorf("failed to decode lp storage: response=invalid")
		}
		for i := range part {
			out = append(out, append([]byte(nil), b[64+i*32:96+i*32]...))
		}
	}
	return out, nil
}

func (c *capture) getters(p Pool, calls [][]byte) ([][]byte, error) {
	out := make([][]byte, 0, len(calls))
	if p.Multicall == (common.Address{}) {
		for _, data := range calls {
			b, err := c.call(p.Address, data, 1)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
		}
		return out, nil
	}
	for start := 0; start < len(calls); start += c.reader.limits.ChunkSize {
		part := calls[start:min(start+c.reader.limits.ChunkSize, len(calls))]
		heads, tail := []byte{}, []byte{}
		for _, data := range part {
			heads = append(heads, uintWord(32*len(part)+len(tail))...)
			tail = append(tail, common.LeftPadBytes(p.Address[:], 32)...)
			tail = append(tail, uintWord(64)...)
			tail = append(tail, uintWord(len(data))...)
			tail = append(tail, data...)
			tail = append(tail, make([]byte, (32-len(data)%32)%32)...)
		}
		data := append(selector("tryAggregate(bool,(address,bytes)[])"), uintWord(1)...)
		data = append(data, uintWord(64)...)
		data = append(data, uintWord(len(part))...)
		data = append(data, heads...)
		data = append(data, tail...)
		b, err := c.call(p.Multicall, data, len(part))
		if err != nil {
			return nil, err
		}
		// ABI decoding validates dynamic offsets and lengths before slicing.
		typ, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{{Name: "success", Type: "bool"}, {Name: "returnData", Type: "bytes"}})
		if err != nil {
			return nil, fmt.Errorf("failed to compose lp multicall decoder: %w", err)
		}
		values, err := (abi.Arguments{{Type: typ}}).Unpack(b)
		if err != nil {
			return nil, fmt.Errorf("failed to decode lp multicall: %w", err)
		}
		if len(values) != 1 {
			return nil, fmt.Errorf("failed to decode lp multicall: result=invalid")
		}
		rows, ok := values[0].([]struct {
			Success    bool   `json:"success"`
			ReturnData []byte `json:"returnData"`
		})
		if !ok || len(rows) != len(part) {
			return nil, fmt.Errorf("failed to decode lp multicall: results=invalid")
		}
		for _, row := range rows {
			if !row.Success {
				return nil, fmt.Errorf("failed to read lp multicall: call=failed")
			}
			out = append(out, append([]byte(nil), row.ReturnData...))
		}
	}
	return out, nil
}
