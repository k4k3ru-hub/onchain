package v3

import (
	"context"
	"fmt"
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
	BidAmountOut, AskAmountIn *big.Int
	BlockNumber               uint64
	BlockHash                 common.Hash
	ObservedAt                time.Time
}

type poolSnapshot struct {
	header           evm.BlockHeader
	observed         time.Time
	price, liquidity *big.Int
	tick, spacing    int32
	words            map[int32]*big.Int
	ticks            map[int32]*big.Int
}

type StateCache struct {
	rpc              StateRPC
	pool             common.Address
	fee              uint32
	maxAge           time.Duration
	gate             chan struct{}
	mu               sync.Mutex
	generation       uint64
	active           bool
	running          bool
	floor            uint64
	covered          evm.BlockHeader // protected by mu
	spacing          int32           // immutable after a verified snapshot; protected by gate
	snapshot         *poolSnapshot   // protected by gate
	cachedGeneration uint64
}

// NewStateCache creates a bounded-age, block-coherent local quote cache for a configured pool.
//
// Version:
//   - 2026-09-07: Added.
func NewStateCache(client *HTTPClient, rpc StateRPC, pool common.Address, maxAge time.Duration) (*StateCache, error) {
	if client == nil || rpc == nil {
		return nil, fmt.Errorf("failed to create uniswap v3 state cache: dependency=null")
	}
	key, ok := client.poolByAdd[pool]
	if !ok || key.Fee >= 1000000 || maxAge <= 0 {
		return nil, fmt.Errorf("failed to create uniswap v3 state cache: configuration=invalid")
	}
	c := &StateCache{rpc: rpc, pool: pool, fee: key.Fee, maxAge: maxAge, gate: make(chan struct{}, 1)}
	c.gate <- struct{}{}
	return c, nil
}

// Run invalidates on pool logs, including Mint/Burn and removed logs, until disconnect.
// Non-removed logs from the exact verified snapshot block are already reflected in state.
// Reconnection and disconnect both invalidate the snapshot; without a subscription every quote refreshes.
//
// Version:
//   - 2026-09-07: Ignore non-removed logs already covered by the verified block.
func (c *StateCache) Run(ctx context.Context, ws WSRPCClient) error {
	if ctx == nil || ws == nil {
		return fmt.Errorf("failed to watch uniswap v3 state: dependency=null")
	}
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("failed to watch uniswap v3 state: subscription=active")
	}
	c.running = true
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.running = false; c.mu.Unlock() }()
	logs := make(chan types.Log, 256)
	sub, err := ws.SubscribeFilterLogs(ctx, ethereum.FilterQuery{Addresses: []common.Address{c.pool}}, logs)
	if err != nil {
		return fmt.Errorf("failed to watch uniswap v3 state: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("failed to watch uniswap v3 state: subscription=null")
	}
	defer sub.Unsubscribe()
	c.mu.Lock()
	c.active = true
	c.generation++
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.active = false; c.generation++; c.mu.Unlock() }()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-sub.Err():
			if !ok || err == nil {
				return fmt.Errorf("failed to watch uniswap v3 state: subscription=closed")
			}
			return fmt.Errorf("failed to watch uniswap v3 state: %w", err)
		case log, ok := <-logs:
			if !ok {
				return fmt.Errorf("failed to watch uniswap v3 state: logs=closed")
			}
			if log.Address != c.pool {
				continue
			}
			c.mu.Lock()
			if !log.Removed && log.BlockHash != (common.Hash{}) && log.BlockNumber == c.covered.Number && log.BlockHash == c.covered.Hash {
				c.mu.Unlock()
				continue
			}
			c.generation++
			if log.BlockNumber > c.floor {
				c.floor = log.BlockNumber
			}
			c.mu.Unlock()
		}
	}
}

// QuotePair quotes a base exact-input bid and opposite exact-output ask on one snapshot.
// Verified immutable tick spacing is reused across snapshots; lagging heads fail before contract reads.
// Snapshot reads are bounded to 64 contract calls per pair and 2048 swap steps per direction.
//
// Version:
//   - 2026-09-07: Reuse verified immutable spacing and reject lag before state reads.
func (c *StateCache) QuotePair(ctx context.Context, baseAmount *big.Int, baseIsToken0 bool) (LocalPair, error) {
	if ctx == nil || baseAmount == nil || baseAmount.Sign() <= 0 || baseAmount.BitLen() > 255 {
		return LocalPair{}, fmt.Errorf("failed to quote uniswap v3 state: amount=out_of_range")
	}
	select {
	case <-ctx.Done():
		return LocalPair{}, ctx.Err()
	case <-c.gate:
	}
	defer func() { c.gate <- struct{}{} }()
	c.mu.Lock()
	gen, active, floor := c.generation, c.active, c.floor
	c.mu.Unlock()
	budget := 64
	s := c.snapshot
	if s == nil || !active || gen != c.cachedGeneration || time.Since(s.observed) >= c.maxAge {
		var err error
		s, err = c.capture(ctx, &budget, floor)
		if err != nil {
			c.snapshot = nil
			return LocalPair{}, err
		}
	}
	if s.header.Number < floor {
		c.snapshot = nil
		return LocalPair{}, fmt.Errorf("failed to quote uniswap v3 state: rpc_head=behind")
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
			return LocalPair{}, fmt.Errorf("failed to verify uniswap v3 state: %w", err)
		}
		if header.Hash != s.header.Hash {
			c.snapshot = nil
			return LocalPair{}, fmt.Errorf("failed to verify uniswap v3 state: block_hash=mismatch")
		}
	}
	c.mu.Lock()
	changed := gen != c.generation
	if !changed {
		c.covered = s.header
	}
	c.mu.Unlock()
	if changed || time.Since(s.observed) >= c.maxAge {
		c.snapshot = nil
		return LocalPair{}, fmt.Errorf("failed to quote uniswap v3 state: snapshot=invalidated")
	}
	c.snapshot = s
	c.spacing = s.spacing
	c.cachedGeneration = gen
	return LocalPair{BidAmountOut: bid, AskAmountIn: ask, BlockNumber: s.header.Number, BlockHash: s.header.Hash, ObservedAt: s.observed}, nil
}

func (c *StateCache) read(ctx context.Context, s *poolSnapshot, sig string, arg *big.Int, budget *int) ([]byte, error) {
	if *budget <= 0 {
		return nil, fmt.Errorf("failed to read uniswap v3 state: rpc_budget=exhausted")
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
		return nil, fmt.Errorf("failed to read uniswap v3 state: %w", err)
	}
	return result, nil
}
func (c *StateCache) capture(ctx context.Context, budget *int, floor uint64) (*poolSnapshot, error) {
	header, err := c.rpc.LatestHeader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to capture uniswap v3 state: %w", err)
	}
	if header.Hash == (common.Hash{}) {
		return nil, fmt.Errorf("failed to capture uniswap v3 state: block_hash=empty")
	}
	if header.Number < floor {
		return nil, fmt.Errorf("failed to quote uniswap v3 state: rpc_head=behind")
	}
	s := &poolSnapshot{header: header, observed: time.Now().UTC(), words: make(map[int32]*big.Int), ticks: make(map[int32]*big.Int)}
	data, err := c.read(ctx, s, "slot0()", nil, budget)
	if err != nil {
		return nil, err
	}
	slot, err := decodeSlot0(data)
	if err != nil {
		return nil, err
	}
	if !slot.Unlocked || slot.Tick < -887272 || slot.Tick > 887272 || slot.SqrtPriceX96.Cmp(sqrtAtTick(-887272)) < 0 || slot.SqrtPriceX96.Cmp(sqrtAtTick(887272)) >= 0 {
		return nil, fmt.Errorf("failed to capture uniswap v3 state: slot0=invalid")
	}
	s.price = slot.SqrtPriceX96
	s.tick = slot.Tick
	data, err = c.read(ctx, s, "liquidity()", nil, budget)
	if err != nil {
		return nil, err
	}
	if len(data) != 32 {
		return nil, fmt.Errorf("failed to capture uniswap v3 state: liquidity=invalid")
	}
	s.liquidity = new(big.Int).SetBytes(data)
	if s.liquidity.BitLen() > 128 {
		return nil, fmt.Errorf("failed to capture uniswap v3 state: liquidity=out_of_range")
	}
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
		return nil, fmt.Errorf("failed to capture uniswap v3 state: tick_spacing=invalid")
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
			return 0, false, fmt.Errorf("failed to read uniswap v3 bitmap: result=invalid")
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
	if len(data) != 256 || new(big.Int).SetBytes(data[224:]).Cmp(big.NewInt(1)) != 0 {
		return nil, fmt.Errorf("failed to read uniswap v3 tick: result=invalid")
	}
	n := new(big.Int).SetBytes(data[32:64])
	if n.Bit(255) != 0 {
		n.Sub(n, power2(256))
	}
	if n.Cmp(new(big.Int).Neg(power2(127))) < 0 || n.Cmp(power2(127)) >= 0 {
		return nil, fmt.Errorf("failed to read uniswap v3 tick: liquidity_net=out_of_range")
	}
	s.ticks[tick] = n
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
			return nil, fmt.Errorf("failed to quote uniswap v3 state: %w", err)
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
		step, err := calculateStep(p, target, l, remaining, c.fee, input, zero)
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
			return nil, fmt.Errorf("failed to quote uniswap v3 state: amount=out_of_range")
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
					return nil, fmt.Errorf("failed to quote uniswap v3 state: liquidity=out_of_range")
				}
			}
			tick = next
			if zero {
				tick--
			}
		} else {
			return nil, fmt.Errorf("failed to quote uniswap v3 state: incomplete_step=invalid")
		}
	}
	return nil, fmt.Errorf("failed to quote uniswap v3 state: liquidity_or_step_budget=insufficient")
}
