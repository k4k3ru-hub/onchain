package slipstream

import (
	"context"
	"errors"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type StateRPC interface {
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
	LatestHeader(context.Context) (evm.BlockHeader, error)
	HeaderByNumber(context.Context, uint64) (evm.BlockHeader, error)
}

type LocalPair struct {
	ReceivedAt                time.Time
	CapturedAt                time.Time
	BidAmountOut, AskAmountIn *big.Int
	BlockNumber               uint64
	BlockHash                 common.Hash
	ObservedAt                time.Time
	FeePPM                    uint32
}

type poolSnapshot struct {
	windowCenter     int32
	windowCaptured   bool
	header           evm.BlockHeader
	observed         time.Time
	price, liquidity *big.Int
	tick, spacing    int32
	fee              uint32
	words            map[int32]*big.Int
	ticks            map[int32]*big.Int
	gross            map[int32]*big.Int
}

type quoteNotifications struct {
	count                uint64
	block                evm.BlockHeader
	unsafe               bool
	minBlock, maxBlock   uint64
	removed, missingHash bool
}

type StateCache struct {
	quoteSnapshotObserver func(*QuoteSnapshot)
	replayBase            *retainedPoolState
	replayState           *retainedPoolState
	replayLogs            []types.Log
	retained              *retainedPoolState // protected by mu; independent of the legacy quote gate
	verificationEpoch     uint64
	rpc                   StateRPC
	pool                  common.Address
	factory               common.Address
	maxAge                time.Duration
	gate                  chan struct{}
	mu                    sync.Mutex
	generation            uint64
	updates               chan struct{}
	diagnostics           map[string]uint64   // cumulative bounded-label counts; protected by mu
	notifications         *quoteNotifications // active quote only; protected by mu
	active                bool
	running               bool
	floor                 uint64
	floorHash             common.Hash     // non-removed observed block; protected by mu
	covered               evm.BlockHeader // protected by mu
	spacing               int32           // immutable after a verified snapshot; protected by gate
	snapshot              *poolSnapshot   // protected by gate
	cachedGeneration      uint64
}

// NewStateCache creates a bounded-age, block-coherent local quote cache for a configured pool.
// The caller supplies the pool's factory for invalidation. Fee-module changes without
// pool/factory logs are observed on expiry; reused quotes retain their original observation time.
//
// Version:
//   - 2026-09-08: Added.
func NewStateCache(rpc StateRPC, pool, factory common.Address, maxAge time.Duration) (*StateCache, error) {
	if rpc == nil {
		return nil, fmt.Errorf("failed to create slipstream state cache: dependency=null")
	}
	if pool == (common.Address{}) || factory == (common.Address{}) || maxAge <= 0 {
		return nil, fmt.Errorf("failed to create slipstream state cache: configuration=invalid")
	}
	c := &StateCache{rpc: rpc, pool: pool, factory: factory, maxAge: maxAge, gate: make(chan struct{}, 1)}
	c.gate <- struct{}{}
	return c, nil
}

// Run invalidates on pool and factory logs, including Mint/Burn and removed logs, until disconnect.
// Non-removed logs from the exact verified snapshot block are already reflected in state.
// Reconnection and disconnect both invalidate the snapshot; without a subscription every quote refreshes.
//
// Version:
//   - 2026-09-09: Support independent quote-state verification and freshness.
//   - 2026-09-08: Count notification sources and block relationships without changing invalidation.
func (c *StateCache) Run(ctx context.Context, ws WSRPCClient) error {
	if c == nil || ctx == nil || ws == nil {
		return fmt.Errorf("failed to watch slipstream state: dependency=null")
	}
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("failed to watch slipstream state: subscription=active")
	}
	c.running = true
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.running = false; c.mu.Unlock() }()
	logs := make(chan types.Log, 256)
	sub, err := ws.SubscribeFilterLogs(ctx, ethereum.FilterQuery{Addresses: []common.Address{c.pool, c.factory}}, logs)
	if err != nil {
		return fmt.Errorf("failed to watch slipstream state: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("failed to watch slipstream state: subscription=null")
	}
	defer sub.Unsubscribe()
	c.mu.Lock()
	c.active = true
	c.floorHash = common.Hash{}
	c.generation++
	c.signalChange()
	c.countDiagnostic("lifecycle.connected")
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.active = false
		c.floorHash = common.Hash{}
		c.generation++
		c.signalChange()
		c.countDiagnostic("lifecycle.disconnected")
		c.mu.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-sub.Err():
			if !ok || err == nil {
				return fmt.Errorf("failed to watch slipstream state: subscription=closed")
			}
			return fmt.Errorf("failed to watch slipstream state: %w", err)
		case log, ok := <-logs:
			if !ok {
				return fmt.Errorf("failed to watch slipstream state: logs=closed")
			}
			if log.Address != c.pool && log.Address != c.factory {
				continue
			}
			c.mu.Lock()
			c.recordNotification(log)
			if !log.Removed && log.BlockHash != (common.Hash{}) && log.BlockNumber == c.covered.Number && log.BlockHash == c.covered.Hash {
				c.mu.Unlock()
				continue
			}
			c.generation++
			c.signalChange()
			if n := c.notifications; n != nil {
				n.count++
				if n.count == 1 {
					n.minBlock = log.BlockNumber
					n.maxBlock = log.BlockNumber
				} else {
					n.minBlock = min(n.minBlock, log.BlockNumber)
					n.maxBlock = max(n.maxBlock, log.BlockNumber)
				}
				n.removed = n.removed || log.Removed
				n.missingHash = n.missingHash || log.BlockHash == (common.Hash{})
				h := evm.BlockHeader{Number: log.BlockNumber, Hash: log.BlockHash}
				if log.Removed || log.BlockHash == (common.Hash{}) || (n.count > 1 && (n.block.Number != h.Number || n.block.Hash != h.Hash)) {
					n.unsafe = true
				}
				n.block = h
			}
			if log.BlockNumber >= c.floor {
				c.floor = log.BlockNumber
				c.floorHash = log.BlockHash
			}
			if log.Removed {
				c.verificationEpoch++
				c.floorHash = common.Hash{}
			}
			c.mu.Unlock()
		}
	}
}

// QuotePair quotes a base exact-input bid and opposite exact-output ask on one snapshot.
// Lagging latest headers use an observed block only after its number and hash are verified.
// Notifications received during capture are accepted only when every change belongs
// to the captured block and hash; removals and lifecycle changes still invalidate.
// Verified immutable tick spacing is reused across snapshots.
// Snapshot reads are bounded to 64 contract calls per pair and 2048 swap steps per direction.
//
// Version:
//   - 2026-09-10: Preserve input receipt time.
//   - 2026-09-09: Classify state-change retries separately from transport failures.
//   - 2026-09-08: Batch independent core-state reads when the injected reader supports it.
func (c *StateCache) QuotePair(ctx context.Context, baseAmount *big.Int, baseIsToken0 bool) (LocalPair, error) {
	if c == nil || ctx == nil || baseAmount == nil || baseAmount.Sign() <= 0 || baseAmount.BitLen() > 255 {
		return LocalPair{}, fmt.Errorf("failed to quote slipstream state: amount=out_of_range")
	}
	select {
	case <-ctx.Done():
		return LocalPair{}, ctx.Err()
	case <-c.gate:
	}
	defer func() { c.gate <- struct{}{} }()
	c.mu.Lock()
	gen, active, floor, floorHash := c.generation, c.active, c.floor, c.floorHash
	notifications := &quoteNotifications{}
	c.notifications = notifications
	if !active {
		floorHash = common.Hash{}
	}
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.notifications = nil; c.mu.Unlock() }()
	budget := 64
	s := c.snapshot
	if s == nil || !active || gen != c.cachedGeneration || time.Since(s.observed) >= c.maxAge {
		c.mu.Lock()
		switch {
		case s == nil:
			c.countDiagnostic("refresh.no_snapshot")
		case !active:
			c.countDiagnostic("refresh.subscription_inactive")
		case gen != c.cachedGeneration:
			c.countDiagnostic("refresh.notifications_or_lifecycle")
		default:
			c.countDiagnostic("refresh.expired")
		}
		c.mu.Unlock()
		var err error
		s, err = c.capture(ctx, &budget, floor, floorHash)
		if err != nil {
			c.snapshot = nil
			return LocalPair{}, err
		}
	}
	if s.header.Number < floor {
		c.snapshot = nil
		return LocalPair{}, fmt.Errorf("failed to quote slipstream state: %w: rpc_head=behind", quotestate.ErrStateChanged)
	}
	bid, err := c.quote(ctx, s, baseAmount, baseIsToken0, true, &budget)
	if err != nil {
		c.snapshot = nil
		return LocalPair{}, err
	}
	ask, err := c.quote(ctx, s, baseAmount, !baseIsToken0, false, &budget)
	if err != nil {
		c.snapshot = nil
		return LocalPair{}, err
	}
	if budget < 64 {
		header, err := c.rpc.HeaderByNumber(ctx, s.header.Number)
		if err != nil {
			c.snapshot = nil
			return LocalPair{}, fmt.Errorf("failed to verify slipstream state: %w", err)
		}
		if header.Number != s.header.Number || header.Hash != s.header.Hash {
			c.snapshot = nil
			return LocalPair{}, fmt.Errorf("failed to verify slipstream state: block_hash=mismatch")
		}
	}
	c.mu.Lock()
	changed := gen != c.generation
	// Lifecycle changes increment generation without recording a log. Require
	// every intervening change to be a non-removed notification in this block.
	if changed && active && c.active && !notifications.unsafe && notifications.count == c.generation-gen && notifications.block.Number == s.header.Number && notifications.block.Hash == s.header.Hash {
		changed = false
		c.countDiagnostic("quote.concurrent_notifications_accepted")
		gen = c.generation
	}
	if changed {
		c.recordInvalidation(notifications, c.generation-gen, s.header)
	}
	if !changed {
		c.covered = s.header
	}
	c.mu.Unlock()
	if changed || time.Since(s.observed) >= c.maxAge {
		if !changed {
			c.mu.Lock()
			c.countDiagnostic("invalidation.expired")
			c.mu.Unlock()
		}
		c.snapshot = nil
		return LocalPair{}, fmt.Errorf("failed to quote slipstream state: %w: snapshot=invalidated", quotestate.ErrStateChanged)
	}
	c.snapshot = s
	c.spacing = s.spacing
	c.cachedGeneration = gen
	return LocalPair{ReceivedAt: s.observed, BidAmountOut: bid, AskAmountIn: ask, BlockNumber: s.header.Number, BlockHash: s.header.Hash, ObservedAt: s.observed, FeePPM: s.fee}, nil
}

var errStateReadBudget = errors.New("rpc_budget=exhausted")

func (c *StateCache) read(ctx context.Context, s *poolSnapshot, sig string, arg *big.Int, budget *int) ([]byte, error) {
	if *budget <= 0 {
		return nil, fmt.Errorf("failed to read slipstream state: %w", errStateReadBudget)
	}
	*budget--
	data := append([]byte(nil), crypto.Keccak256([]byte(sig))[:4]...)
	if arg != nil {
		word := new(big.Int).Set(arg)
		if word.Sign() < 0 {
			word.Add(word, power2(256))
		}
		data = append(data, word.FillBytes(make([]byte, 32))...)
	}
	result, err := c.rpc.CallContract(ctx, ethereum.CallMsg{To: &c.pool, Data: data}, new(big.Int).SetUint64(s.header.Number))
	if err != nil {
		return nil, fmt.Errorf("failed to read slipstream state: %w", err)
	}
	return result, nil
}
func (c *StateCache) capture(ctx context.Context, budget *int, floor uint64, floorHash common.Hash) (*poolSnapshot, error) {
	header, err := c.rpc.LatestHeader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to capture slipstream state: %w", err)
	}
	if header.Hash == (common.Hash{}) {
		return nil, fmt.Errorf("failed to capture slipstream state: block_hash=empty")
	}
	if header.Number < floor {
		if floorHash == (common.Hash{}) {
			return nil, fmt.Errorf("failed to quote slipstream state: %w: rpc_head=behind", quotestate.ErrStateChanged)
		}
		header, err = c.rpc.HeaderByNumber(ctx, floor)
		if err != nil {
			return nil, fmt.Errorf("failed to capture slipstream observed block: %w", err)
		}
		if header.Number != floor || header.Hash != floorHash {
			return nil, fmt.Errorf("failed to capture slipstream observed block: block=mismatch")
		}
	}
	s := &poolSnapshot{header: header, observed: time.Now().UTC(), words: make(map[int32]*big.Int), ticks: make(map[int32]*big.Int)}
	values, err := c.readCoreState(ctx, s, budget)
	data := values[0]
	if err != nil {
		return nil, err
	}
	slot, err := decodeSlot0(data)
	if err != nil {
		return nil, err
	}
	if !slot.Unlocked || slot.Tick < -887272 || slot.Tick > 887272 || slot.SqrtPriceX96.Cmp(sqrtAtTick(-887272)) < 0 || slot.SqrtPriceX96.Cmp(sqrtAtTick(887272)) >= 0 {
		return nil, fmt.Errorf("failed to capture slipstream state: slot0=invalid")
	}
	s.price = slot.SqrtPriceX96
	s.tick = slot.Tick
	data = values[1]
	if len(data) != 32 {
		return nil, fmt.Errorf("failed to capture slipstream state: liquidity=invalid")
	}
	s.liquidity = new(big.Int).SetBytes(data)
	if s.liquidity.BitLen() > 128 {
		return nil, fmt.Errorf("failed to capture slipstream state: liquidity=out_of_range")
	}
	// CLPool captures fee() once at swap start, before crossing any ticks.
	// It calls the factory from the pool; do not substitute a fixed tick-spacing fee.
	data = values[2]
	fee, err := decodeUnsignedWord(data, 24, "fee")
	if err != nil {
		return nil, err
	}
	if fee.Uint64() >= 1_000_000 {
		return nil, fmt.Errorf("failed to capture slipstream state: fee=out_of_range")
	}
	s.fee = uint32(fee.Uint64())
	if c.spacing > 0 {
		s.spacing = c.spacing
		return s, nil
	}
	data, err = c.read(ctx, s, "tickSpacing()", nil, budget)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(data)
	if len(data) != 32 || !n.IsInt64() || n.Int64() <= 0 || n.Int64() > 16383 {
		return nil, fmt.Errorf("failed to capture slipstream state: tick_spacing=invalid")
	}
	s.spacing = int32(n.Int64())
	return s, nil
}

func (c *StateCache) nextTick(ctx context.Context, s *poolSnapshot, tick int32, zero bool, budget *int) (int32, bool, error) {
	compressed := tick / s.spacing
	if tick < 0 && tick%s.spacing != 0 {
		compressed--
	}
	if !zero {
		compressed++
	}
	word := compressed >> 8
	bit := int(compressed & 255)
	bitmap := s.words[word]
	if bitmap == nil {
		data, err := c.read(ctx, s, "tickBitmap(int16)", big.NewInt(int64(word)), budget)
		if err != nil {
			return 0, false, err
		}
		if len(data) != 32 {
			return 0, false, fmt.Errorf("failed to read slipstream bitmap: result=invalid")
		}
		bitmap = new(big.Int).SetBytes(data)
		s.words[word] = bitmap
	}
	step := 1
	end := 255
	if zero {
		step = -1
		end = 0
	}
	for i := bit; i >= 0 && i <= 255; i += step {
		if bitmap.Bit(i) == 1 {
			return (word*256 + int32(i)) * s.spacing, true, nil
		}
	}
	return (word*256 + int32(end)) * s.spacing, false, nil
}
func (c *StateCache) liquidityNet(ctx context.Context, s *poolSnapshot, tick int32, budget *int) (*big.Int, error) {
	if n := s.ticks[tick]; n != nil {
		return n, nil
	}
	data, err := c.read(ctx, s, "ticks(int24)", big.NewInt(int64(tick)), budget)
	if err != nil {
		return nil, err
	}
	return storeTickDetails(s, tick, data)
}

func storeTickDetails(s *poolSnapshot, tick int32, data []byte) (*big.Int, error) {
	if len(data) != 320 || new(big.Int).SetBytes(data[288:]).Cmp(big.NewInt(1)) != 0 {
		return nil, fmt.Errorf("failed to read slipstream tick: result=invalid")
	}
	n := new(big.Int).SetBytes(data[32:64])
	if n.Bit(255) != 0 {
		n.Sub(n, power2(256))
	}
	if n.Cmp(new(big.Int).Neg(power2(127))) < 0 || n.Cmp(power2(127)) >= 0 {
		return nil, fmt.Errorf("failed to read slipstream tick: liquidity_net=out_of_range")
	}
	s.ticks[tick] = n
	gross, err := decodeUnsignedWord(data[:32], 128, "liquidity_gross")
	if err != nil {
		return nil, err
	}
	if s.gross == nil {
		s.gross = make(map[int32]*big.Int)
	}
	s.gross[tick] = gross
	return n, nil
}
func (c *StateCache) quote(ctx context.Context, s *poolSnapshot, amount *big.Int, zero, input bool, budget *int) (*big.Int, error) {
	remaining := new(big.Int).Set(amount)
	result := new(big.Int)
	p := new(big.Int).Set(s.price)
	l := new(big.Int).Set(s.liquidity)
	tick := s.tick
	for count := 0; remaining.Sign() > 0 && count < 2048; count++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("failed to quote slipstream state: %w", err)
		}
		next, initialized, err := c.nextTick(ctx, s, tick, zero, budget)
		if err != nil {
			return nil, err
		}
		next = max(int32(-887272), min(int32(887272), next))
		target := sqrtAtTick(next)
		limit := new(big.Int).Sub(sqrtAtTick(887272), big.NewInt(1))
		if zero {
			limit = new(big.Int).Add(sqrtAtTick(-887272), big.NewInt(1))
		}
		if zero && target.Cmp(limit) < 0 || !zero && target.Cmp(limit) > 0 {
			target = limit
		}
		step, err := calculateStep(p, target, l, remaining, s.fee, input, zero)
		if err != nil {
			return nil, err
		}
		cost := new(big.Int).Add(step.in, step.fee)
		if input {
			remaining.Sub(remaining, cost)
			result.Add(result, step.out)
		} else {
			remaining.Sub(remaining, step.out)
			result.Add(result, cost)
		}
		p = step.price
		if remaining.Sign() < 0 || result.BitLen() > 256 {
			return nil, fmt.Errorf("failed to quote slipstream state: amount=out_of_range")
		}
		if remaining.Sign() == 0 {
			return result, nil
		}
		if p.Cmp(limit) == 0 {
			break
		}
		if p.Cmp(sqrtAtTick(next)) == 0 {
			if initialized {
				net, err := c.liquidityNet(ctx, s, next, budget)
				if err != nil {
					return nil, err
				}
				if zero {
					l.Sub(l, net)
				} else {
					l.Add(l, net)
				}
				if l.Sign() < 0 || l.BitLen() > 128 {
					return nil, fmt.Errorf("failed to quote slipstream state: liquidity=out_of_range")
				}
			}
			tick = next
			if zero {
				tick--
			}
		} else {
			return nil, fmt.Errorf("failed to quote slipstream state: incomplete_step=invalid")
		}
	}
	return nil, fmt.Errorf("failed to quote slipstream state: liquidity_or_step_budget=insufficient")
}

// batchStateReader is optional: injected readers without it retain sequential reads.
type batchStateReader interface {
	ReadContracts(context.Context, common.Address, [][]byte, uint64) ([][]byte, []error, error)
}

func (c *StateCache) readCoreState(ctx context.Context, s *poolSnapshot, budget *int) ([3][]byte, error) {
	var values [3][]byte
	names := []string{"slot0()", "liquidity()", "fee()"}
	if batch, ok := c.rpc.(batchStateReader); ok {
		if *budget < 3 {
			return values, fmt.Errorf("failed to read slipstream state: %w", errStateReadBudget)
		}
		*budget -= 3
		data := make([][]byte, 3)
		for i, name := range names {
			data[i] = crypto.Keccak256([]byte(name))[:4]
		}
		result, failures, err := batch.ReadContracts(ctx, c.pool, data, s.header.Number)
		if err != nil {
			return values, fmt.Errorf("failed to read slipstream state batch: %w", err)
		}
		if len(result) != 3 || len(failures) != 3 {
			return values, fmt.Errorf("failed to read slipstream state batch: result=invalid")
		}
		if err := errors.Join(failures...); err != nil {
			return values, fmt.Errorf("failed to read slipstream state batch: %w", err)
		}
		copy(values[:], result)
		return values, nil
	}
	for i, name := range names {
		v, err := c.read(ctx, s, name, nil, budget)
		if err != nil {
			return values, err
		}
		values[i] = v
	}
	return values, nil
}
