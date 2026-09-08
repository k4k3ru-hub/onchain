package clmm

import (
	"context"
	"encoding/json"
	"fmt"
	sui "github.com/k4k3ru-hub/onchain/go/sui"
	"math/big"
	"sync/atomic"
	"time"
)

type StateReader interface {
	LatestCheckpoint(context.Context) (sui.Checkpoint, error)
	ObjectAtCheckpoint(context.Context, sui.Address, sui.CheckpointSequenceNumber) (*sui.Object, error)
	DynamicValuesAtCheckpoint(context.Context, sui.Address, sui.CheckpointSequenceNumber, int, string) (sui.DynamicValuePage, error)
}

type StateCache struct {
	gate     chan struct{}
	reader   StateReader
	pool     sui.Address
	maxAge   time.Duration
	snapshot *LocalSnapshot
	anchors  []Tick
	version  uint64
	digest   sui.ObjectDigest
	observed time.Time
	floor    atomic.Uint64
}

// NewStateCache composes a checkpoint-pinned Cetus state cache.
// Each quote validates the pool version at a fresh checkpoint before reusing ticks.
//
// Version:
//   - 2026-09-08: Added.
func NewStateCache(reader StateReader, pool sui.Address, maxAge time.Duration) (*StateCache, error) {
	if reader == nil || pool.IsZero() || maxAge <= 0 {
		return nil, fmt.Errorf("failed to create cetus state cache: parameters=invalid")
	}
	return &StateCache{reader: reader, pool: pool, maxAge: maxAge, gate: make(chan struct{}, 1)}, nil
}

// ObserveCheckpoint requires future reads to include an observed pool change.
//
// Version:
//   - 2026-09-08: Added.
func (c *StateCache) ObserveCheckpoint(checkpoint sui.CheckpointSequenceNumber) {
	for {
		previous := c.floor.Load()
		if checkpoint.Uint64() <= previous || c.floor.CompareAndSwap(previous, checkpoint.Uint64()) {
			return
		}
	}
}

// QuotePair computes both sides from a checkpoint-pinned pool and verified tick coverage.
// No simulation fallback or partial fill is returned on an invalid snapshot.
//
// Version:
//   - 2026-09-08: Added.
func (c *StateCache) QuotePair(ctx context.Context, params QuotePairParams) (QuotePairResult, error) {
	if c == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: cache=null")
	}
	if params.Bid.Pool.Address != c.pool || params.Ask.Pool.Address != c.pool || params.Bid.A2B == params.Ask.A2B {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: parameters=invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: %w", ctx.Err())
	}
	defer func() { <-c.gate }()
	head, err := c.reader.LatestCheckpoint(ctx)
	checkpoint := head.SequenceNumber
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: %w", err)
	}
	if head.Timestamp.IsZero() || time.Since(head.Timestamp) > c.maxAge {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: checkpoint=expired")
	}
	if checkpoint.Uint64() < c.floor.Load() {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: checkpoint=behind")
	}
	obj, err := c.reader.ObjectAtCheckpoint(ctx, c.pool, checkpoint)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: %w", err)
	}
	if obj == nil || obj.Version == 0 || obj.Address != c.pool {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: object=invalid")
	}
	if c.snapshot == nil || obj.Version != c.version || obj.Digest != c.digest || time.Since(c.observed) > c.maxAge {
		started := time.Now()
		snapshot, err := captureNeighbors(ctx, c.reader, obj, checkpoint, c.anchors, params)
		if err == nil && snapshot == nil {
			snapshot, err = captureState(ctx, c.reader, obj, checkpoint)
			if err == nil {
				c.anchors = append([]Tick(nil), snapshot.Ticks...)
			}
		}
		if err != nil {
			return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: %w", err)
		}
		c.snapshot = snapshot
		c.version = obj.Version
		c.digest = obj.Digest
		c.observed = started
	}
	bid, err := c.snapshot.Quote(params.Bid.AmountIn, params.Bid.A2B, true)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached bid: %w", err)
	}
	ask, err := c.snapshot.Quote(params.Ask.AmountOut, params.Ask.A2B, false)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached ask: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: %w", err)
	}
	if checkpoint.Uint64() < c.floor.Load() || time.Since(c.observed) > c.maxAge || time.Since(head.Timestamp) > c.maxAge {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus cached state: snapshot=invalidated")
	}
	bid.Checkpoint = checkpoint
	ask.Checkpoint = checkpoint
	return QuotePairResult{Bid: bid, Ask: ask, Checkpoint: checkpoint, PoolVersion: c.version, PoolDigest: c.digest, StateTimestamp: head.Timestamp}, nil
}

func captureState(ctx context.Context, reader StateReader, obj *sui.Object, checkpoint sui.CheckpointSequenceNumber) (*LocalSnapshot, error) {
	pool, err := ParsePool(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to capture cetus state: %w", err)
	}
	var metadata struct {
		Manager struct {
			Ticks struct {
				ID   string          `json:"id"`
				Size json.RawMessage `json:"size"`
			} `json:"ticks"`
		} `json:"tick_manager"`
	}
	if err := json.Unmarshal(obj.Move.JSON, &metadata); err != nil {
		return nil, fmt.Errorf("failed to capture cetus state: %w", err)
	}
	handle, err := sui.ParseAddress(metadata.Manager.Ticks.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to capture cetus state: %w", err)
	}
	size, err := jsonUint64(metadata.Manager.Ticks.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to capture cetus state: %w", err)
	}
	if size > 10000 {
		return nil, fmt.Errorf("failed to capture cetus state: tick_count=out_of_range")
	}
	snapshot := &LocalSnapshot{Pool: *pool}
	cursor := ""
	seen := map[string]bool{}
	indices := map[int32]bool{}
	for {
		page, err := reader.DynamicValuesAtCheckpoint(ctx, handle, checkpoint, 50, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to capture cetus ticks: %w", err)
		}
		for _, value := range page.Values {
			t, err := parseTick(value)
			if err != nil {
				return nil, fmt.Errorf("failed to capture cetus ticks: %w", err)
			}
			if indices[t.Index] {
				return nil, fmt.Errorf("failed to capture cetus ticks: index=duplicate")
			}
			indices[t.Index] = true
			snapshot.Ticks = append(snapshot.Ticks, t)
		}
		if uint64(len(snapshot.Ticks)) > size {
			return nil, fmt.Errorf("failed to capture cetus ticks: count=mismatch")
		}
		if !page.HasNextPage {
			break
		}
		if page.NextCursor == "" || seen[page.NextCursor] || len(page.Values) == 0 {
			return nil, fmt.Errorf("failed to capture cetus ticks: cursor=invalid")
		}
		seen[page.NextCursor] = true
		cursor = page.NextCursor
	}
	if uint64(len(snapshot.Ticks)) != size {
		return nil, fmt.Errorf("failed to capture cetus ticks: count=mismatch")
	}
	return snapshot, nil
}
func parseTick(raw json.RawMessage) (Tick, error) {
	var node struct {
		Value struct {
			Index json.RawMessage `json:"index"`
			Price json.RawMessage `json:"sqrt_price"`
			Net   struct {
				Bits json.RawMessage `json:"bits"`
			} `json:"liquidity_net"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &node); err != nil {
		return Tick{}, fmt.Errorf("failed to parse cetus tick: %w", err)
	}
	index, err := jsonSigned32(node.Value.Index)
	if err != nil {
		return Tick{}, fmt.Errorf("failed to parse cetus tick index: %w", err)
	}
	price, err := jsonUnsigned(node.Value.Price)
	if err != nil {
		return Tick{}, fmt.Errorf("failed to parse cetus tick price: %w", err)
	}
	net, err := jsonUnsigned(node.Value.Net.Bits)
	if err != nil {
		return Tick{}, fmt.Errorf("failed to parse cetus tick liquidity: %w", err)
	}
	if net.BitLen() > 128 {
		return Tick{}, fmt.Errorf("failed to parse cetus tick: liquidity=out_of_range")
	}
	if net.Bit(127) == 1 {
		net.Sub(net, new(big.Int).Lsh(big.NewInt(1), 128))
	}
	return Tick{Index: index, SqrtPrice: price, LiquidityNet: net}, nil
}

// Warm fetches the full checkpoint-pinned tick set outside the latency-sensitive quote path.
// Callers should use a bounded startup or recovery context; no quote is published.
//
// Version:
//   - 2026-09-08: Added.
func (c *StateCache) Warm(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("failed to warm cetus state: cache=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("failed to warm cetus state: %w", ctx.Err())
	}
	defer func() { <-c.gate }()
	started := time.Now()
	head, err := c.reader.LatestCheckpoint(ctx)
	if err != nil {
		return fmt.Errorf("failed to warm cetus state: %w", err)
	}
	obj, err := c.reader.ObjectAtCheckpoint(ctx, c.pool, head.SequenceNumber)
	if err != nil {
		return fmt.Errorf("failed to warm cetus state: %w", err)
	}
	if obj == nil || obj.Address != c.pool || obj.Version == 0 {
		return fmt.Errorf("failed to warm cetus state: object=invalid")
	}
	s, err := captureState(ctx, c.reader, obj, head.SequenceNumber)
	if err != nil {
		return fmt.Errorf("failed to warm cetus state: %w", err)
	}
	c.snapshot = s
	c.anchors = append([]Tick(nil), s.Ticks...)
	c.version = obj.Version
	c.digest = obj.Digest
	c.observed = started
	return nil
}
