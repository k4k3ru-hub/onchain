package spot

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	sui "github.com/k4k3ru-hub/onchain/go/sui"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type StateReader interface {
	LatestCheckpoint(context.Context) (sui.Checkpoint, error)
	ObjectAtCheckpoint(context.Context, sui.Address, sui.CheckpointSequenceNumber) (*sui.Object, error)
	DynamicValuesByKeysAtCheckpoint(context.Context, sui.Address, sui.CheckpointSequenceNumber, []sui.DynamicFieldKey) ([]json.RawMessage, error)
}
type StateCache struct {
	quoteSnapshotObserver func(*QuoteSnapshot)
	retainedMu            sync.Mutex
	retained              *quoteState
	retainedHead          sui.Checkpoint
	retainedReference     *QuotePairParams
	retainedRunning       bool
	checkedKey            string
	accepted              sui.Checkpoint // Protected by gate; independent of trade progress.
	reader                StateReader
	pool                  sui.Address
	maxAge                time.Duration
	gate                  chan struct{}
	floor                 atomic.Uint64
	state                 *quoteState
}
type quoteState struct {
	position      *quotestate.Position
	received      time.Time
	retainedOnly  bool
	pool          Pool
	tickIndex     int32
	spacing       uint32
	version       uint64
	digest        sui.ObjectDigest
	checkpoint    sui.CheckpointSequenceNumber
	captured      time.Time
	bitmap, ticks sui.Address
	keyType       string
	protocolRate  uint64
	words         map[int32]*big.Int
	nets          map[int32]*big.Int
}

// NewStateCache composes a checkpoint-pinned Bluefin pool cache with lazy bitmap and tick reads.
//
// Version:
//   - 2026-09-08: Added.
func NewStateCache(reader StateReader, pool sui.Address, maxAge time.Duration) (*StateCache, error) {
	if reader == nil || pool.IsZero() || maxAge <= 0 {
		return nil, fmt.Errorf("failed to create bluefin state cache: parameters=invalid")
	}
	return &StateCache{reader: reader, pool: pool, maxAge: maxAge, gate: make(chan struct{}, 1)}, nil
}

// ObserveCheckpoint requires subsequent quotes to include the observed pool change.
//
// Version:
//   - 2026-09-08: Added.
func (c *StateCache) ObserveCheckpoint(cp sui.CheckpointSequenceNumber) {
	for {
		old := c.floor.Load()
		if cp.Uint64() <= old || c.floor.CompareAndSwap(old, cp.Uint64()) {
			return
		}
	}
}

// QuotePair computes bid and ask against the same pool, bitmap and tick checkpoint.
// AmountIn includes fees. FeeAmount excludes the separately reported protocol share. Partial fills are rejected.
//
// Version:
//   - 2026-09-10: Preserve input receipt time.
//   - 2026-09-09: Publish detached calculation inputs to the snapshot observer.
//   - 2026-09-11: Retain the reference quantity for producer-side coverage recovery.
//   - 2026-09-09: Seed detached retained inputs after verified initialization.
//   - 2026-09-08: Reject checkpoint regression relative to successful quotes.
//   - 2026-09-08: Added.
func (c *StateCache) QuotePair(ctx context.Context, p QuotePairParams) (QuotePairResult, error) {
	if c == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: cache=null")
	}
	if p.Bid.Pool.Address != c.pool || p.Ask.Pool.Address != c.pool || p.Bid.A2B == p.Ask.A2B {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: parameters=invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: %w", ctx.Err())
	}
	defer func() { <-c.gate }()
	head, err := sui.QuoteCheckpoint(ctx, c.reader, sui.CheckpointSequenceNumber(c.floor.Load()))
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: %w", err)
	}
	if head.Timestamp.IsZero() || time.Since(head.Timestamp) > c.maxAge {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: checkpoint=invalid")
	}
	if head.SequenceNumber.Uint64() < c.floor.Load() {
		return QuotePairResult{}, fmt.Errorf("failed to verify quote checkpoint: %w: checkpoint=behind", quotestate.ErrStateChanged)
	}
	if head.SequenceNumber < c.accepted.SequenceNumber || head.Timestamp.Before(c.accepted.Timestamp) {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: checkpoint=regressed")
	}
	if head.SequenceNumber == c.accepted.SequenceNumber && head.Digest != c.accepted.Digest {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: checkpoint_digest=mismatch")
	}

	obj, err := c.reader.ObjectAtCheckpoint(ctx, c.pool, head.SequenceNumber)
	receivedAt := time.Now().UTC()
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: %w", err)
	}
	if obj == nil || obj.Address != c.pool || obj.Version == 0 {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: object=invalid")
	}
	if c.state == nil || obj.Version != c.state.version || obj.Digest != c.state.digest || time.Since(c.state.captured) > c.maxAge {
		state, err := capturePool(obj, head.SequenceNumber)
		if err != nil {
			return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: %w", err)
		}
		c.state = state
	}
	bid, err := c.quote(ctx, c.state, p.Bid.AmountIn, p.Bid.A2B, true, p.Bid.SqrtPriceLimit)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached bid: %w", err)
	}
	ask, err := c.quote(ctx, c.state, p.Ask.AmountOut, p.Ask.A2B, false, p.Ask.SqrtPriceLimit)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached ask: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: %w", err)
	}
	if head.SequenceNumber.Uint64() < c.floor.Load() || time.Since(head.Timestamp) > c.maxAge || time.Since(c.state.captured) > c.maxAge {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin cached state: %w: snapshot=invalidated", quotestate.ErrStateChanged)
	}
	bid.Checkpoint = head.SequenceNumber
	ask.Checkpoint = head.SequenceNumber
	c.accepted = head
	c.retainedMu.Lock()
	c.state.received = receivedAt
	c.retained = cloneRetainedState(c.state)
	c.retained.retainedOnly = true
	c.retainedHead = head
	reference := p
	if p.Bid.SqrtPriceLimit != nil {
		reference.Bid.SqrtPriceLimit = new(big.Int).Set(p.Bid.SqrtPriceLimit)
	}
	if p.Ask.SqrtPriceLimit != nil {
		reference.Ask.SqrtPriceLimit = new(big.Int).Set(p.Ask.SqrtPriceLimit)
	}
	c.retainedReference = &reference
	c.publishQuoteSnapshotLocked()
	c.retainedMu.Unlock()
	return QuotePairResult{ReceivedAt: c.state.received, Bid: bid, Ask: ask, Checkpoint: head.SequenceNumber, PoolVersion: obj.Version, PoolDigest: obj.Digest, StateTimestamp: head.Timestamp}, nil
}
func capturePool(obj *sui.Object, cp sui.CheckpointSequenceNumber) (*quoteState, error) {
	if obj == nil || obj.Move == nil {
		return nil, fmt.Errorf("failed to capture bluefin pool: object=null")
	}
	// The integer key package is tied to the supported original Bluefin deployment.
	if !strings.HasPrefix(obj.Move.Type, bluefinPoolPackage+"::pool::Pool<") {
		return nil, fmt.Errorf("failed to capture bluefin pool: deployment=unsupported")
	}

	p, err := ParsePool(obj)
	if err != nil {
		return nil, err
	}
	if p.FeeRate >= 1000000 || p.Liquidity == nil || p.Liquidity.Sign() < 0 || p.Liquidity.BitLen() > 128 || p.SqrtPrice == nil || p.SqrtPrice.Cmp(sqrtAtTick(-443636)) < 0 || p.SqrtPrice.Cmp(sqrtAtTick(443636)) > 0 {
		return nil, fmt.Errorf("failed to capture bluefin pool: state=invalid")
	}

	var fields struct {
		Manager struct {
			Spacing uint32 `json:"tick_spacing"`
			Ticks   struct {
				ID string `json:"id"`
			} `json:"ticks"`
			Bitmap struct {
				ID string `json:"id"`
			} `json:"bitmap"`
		} `json:"ticks_manager"`
		Index struct {
			Bits json.RawMessage `json:"bits"`
		} `json:"current_tick_index"`
		Protocol json.RawMessage `json:"protocol_fee_share"`
	}
	if err := json.Unmarshal(obj.Move.JSON, &fields); err != nil {
		return nil, fmt.Errorf("failed to capture bluefin pool: %w", err)
	}
	index, err := jsonUint64(fields.Index.Bits)
	if err != nil {
		return nil, fmt.Errorf("failed to capture bluefin index: %w", err)
	}
	protocol, err := jsonUint64(fields.Protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to capture bluefin protocol share: %w", err)
	}
	tick := int32(uint32(index))
	if p.Paused || protocol > 1000000 || index > uint64(^uint32(0)) || tick < -443636 || tick > 443636 || fields.Manager.Spacing == 0 || fields.Manager.Spacing > 443636 {
		return nil, fmt.Errorf("failed to capture bluefin pool: state=invalid")
	}
	bitmap, err := sui.ParseAddress(fields.Manager.Bitmap.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to capture bluefin bitmap: %w", err)
	}
	ticks, err := sui.ParseAddress(fields.Manager.Ticks.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to capture bluefin ticks: %w", err)
	}
	return &quoteState{pool: *p, tickIndex: tick, spacing: fields.Manager.Spacing, protocolRate: protocol, version: obj.Version, digest: obj.Digest, checkpoint: cp, captured: time.Now(), bitmap: bitmap, ticks: ticks, keyType: integerKeyType, words: map[int32]*big.Int{}, nets: map[int32]*big.Int{}}, nil
}

const bluefinPoolPackage = "0x3492c874c1e3b3e2984e8c41b589e642d4d0a5d6459e5a9cfc2d52fd7c89c267"

const integerKeyType = "0x714a63a0dba6da4f017b42d5d0fb78867f18bcde904868e51d951a5a6f5b7f57::i32::I32"

func validU128(n *big.Int) bool { return n != nil && n.Sign() > 0 && n.BitLen() <= 128 }

func (c *StateCache) field(ctx context.Context, s *quoteState, parent sui.Address, index int32) (json.RawMessage, error) {
	if s.retainedOnly {
		return nil, fmt.Errorf("failed to read retained bluefin field: %w", errRetainedCoverage)
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(index))
	values, err := c.reader.DynamicValuesByKeysAtCheckpoint(ctx, parent, s.checkpoint, []sui.DynamicFieldKey{{Type: s.keyType, BCS: b[:]}})
	if err != nil {
		return nil, fmt.Errorf("failed to read bluefin field: %w", err)
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("failed to read bluefin field: values=invalid")
	}
	return values[0], nil
}
func (c *StateCache) nextTick(ctx context.Context, s *quoteState, tick int32, down bool) (int32, bool, error) {
	spacing := int32(s.spacing)
	compressed := tick / spacing
	if tick < 0 && tick%spacing != 0 {
		compressed--
	}
	if !down {
		compressed++
	}
	word, bit := compressed>>8, int(compressed&255)
	bits, ok := s.words[word]
	if !ok {
		raw, err := c.field(ctx, s, s.bitmap, word)
		if err != nil {
			return 0, false, err
		}
		bits = new(big.Int)
		if len(raw) > 0 {
			bits, err = jsonUnsigned(raw)
			if err != nil {
				return 0, false, fmt.Errorf("failed to parse bluefin bitmap: %w", err)
			}
		}
		if bits.BitLen() > 256 {
			return 0, false, fmt.Errorf("failed to parse bluefin bitmap: word=out_of_range")
		}
		s.words[word] = bits
	}
	end := 255
	if down {
		end = 0
	}
	found := false
	for i := bit; i >= 0 && i < 256; {
		if bits.Bit(i) != 0 {
			end = i
			found = true
			break
		}
		if down {
			i--
		} else {
			i++
		}
	}
	next := (word*256 + int32(end)) * spacing
	if next < -443636 {
		next = -443636
		found = false
	}
	if next > 443636 {
		next = 443636
		found = false
	}
	return next, found, nil
}
func (c *StateCache) net(ctx context.Context, s *quoteState, tick int32) (*big.Int, error) {
	if n, ok := s.nets[tick]; ok {
		return n, nil
	}
	raw, err := c.field(ctx, s, s.ticks, tick)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("failed to read bluefin tick: tick=null")
	}
	var v struct {
		Net struct {
			Bits json.RawMessage `json:"bits"`
		} `json:"liquidity_net"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("failed to parse bluefin tick: %w", err)
	}
	n, err := jsonUnsigned(v.Net.Bits)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bluefin tick: %w", err)
	}
	if n.BitLen() > 128 {
		return nil, fmt.Errorf("failed to parse bluefin tick: liquidity=out_of_range")
	}
	if n.Bit(127) != 0 {
		n.Sub(n, new(big.Int).Lsh(big.NewInt(1), 128))
	}
	s.nets[tick] = n
	return n, nil
}
func (c *StateCache) quote(ctx context.Context, s *quoteState, amount uint64, down, exact bool, limit *big.Int) (QuoteResult, error) {
	if amount == 0 || !validU128(limit) || limit.Cmp(sqrtAtTick(-443636)) < 0 || limit.Cmp(sqrtAtTick(443636)) > 0 || (down && limit.Cmp(s.pool.SqrtPrice) >= 0) || (!down && limit.Cmp(s.pool.SqrtPrice) <= 0) {
		return QuoteResult{}, fmt.Errorf("failed to calculate bluefin quote: parameters=invalid")
	}
	price, liq := new(big.Int).Set(s.pool.SqrtPrice), new(big.Int).Set(s.pool.Liquidity)
	tick := s.tickIndex
	remaining := new(big.Int).SetUint64(amount)
	input, output, fees := new(big.Int), new(big.Int), new(big.Int)
	protocol := new(big.Int)
	for steps := 0; remaining.Sign() > 0 && price.Cmp(limit) != 0; steps++ {
		if err := ctx.Err(); err != nil {
			return QuoteResult{}, fmt.Errorf("failed to calculate bluefin quote: %w", err)
		}
		if steps >= 2048 {
			return QuoteResult{}, fmt.Errorf("failed to calculate bluefin quote: steps=out_of_range")
		}
		next, initialized, err := c.nextTick(ctx, s, tick, down)
		if err != nil {
			return QuoteResult{}, err
		}
		boundary := sqrtAtTick(next)
		target := boundary
		if down && target.Cmp(limit) < 0 || !down && target.Cmp(limit) > 0 {
			target = limit
		}
		in, out, fee := new(big.Int), new(big.Int), new(big.Int)
		newPrice := new(big.Int).Set(target)
		if liq.Sign() > 0 {
			in, out, newPrice, fee, err = localStep(price, target, liq, remaining, s.pool.FeeRate, down, exact)
			if err != nil {
				return QuoteResult{}, err
			}
		}
		if !in.IsUint64() || !out.IsUint64() || !fee.IsUint64() {
			return QuoteResult{}, fmt.Errorf("failed to calculate bluefin quote: amount=out_of_range")
		}
		used := out
		if exact {
			used = new(big.Int).Add(in, fee)
		}
		if used.Cmp(remaining) > 0 {
			return QuoteResult{}, fmt.Errorf("failed to calculate bluefin quote: consumption=out_of_range")
		}
		remaining.Sub(remaining, used)
		input.Add(input, in).Add(input, fee)
		output.Add(output, out)
		fees.Add(fees, fee)
		protocol.Add(protocol, new(big.Int).Quo(new(big.Int).Mul(fee, new(big.Int).SetUint64(s.protocolRate)), big.NewInt(1000000)))
		price = newPrice
		if price.Cmp(boundary) == 0 {
			if initialized {
				net, err := c.net(ctx, s, next)
				if err != nil {
					return QuoteResult{}, err
				}
				if down {
					liq.Sub(liq, net)
				} else {
					liq.Add(liq, net)
				}
				if liq.Sign() < 0 || liq.BitLen() > 128 {
					return QuoteResult{}, fmt.Errorf("failed to calculate bluefin quote: liquidity=out_of_range")
				}
			}
			tick = next
			if down {
				tick--
			}
		} else {
			break
		}
	}
	if remaining.Sign() != 0 {
		return QuoteResult{}, fmt.Errorf("failed to calculate bluefin quote: coverage=insufficient")
	}
	if !input.IsUint64() || !output.IsUint64() || !fees.IsUint64() || output.Sign() == 0 {
		return QuoteResult{}, fmt.Errorf("failed to calculate bluefin quote: amount=out_of_range")
	}
	return QuoteResult{AmountIn: input.Uint64(), AmountOut: output.Uint64(), ProtocolFee: protocol.Uint64(), FeeAmount: new(big.Int).Sub(fees, protocol).Uint64(), AfterSqrtPrice: price}, nil
}
