package clmm

import (
	"context"
	"encoding/json"
	"fmt"
	sui "github.com/k4k3ru-hub/onchain/go/sui"
)

type neighborReader interface {
	DynamicUint64ValuesAtCheckpoint(context.Context, sui.Address, sui.CheckpointSequenceNumber, []uint64) ([]json.RawMessage, error)
}

func captureNeighbors(ctx context.Context, reader StateReader, obj *sui.Object, cp sui.CheckpointSequenceNumber, anchors []Tick, params QuotePairParams) (*LocalSnapshot, error) {
	r, ok := reader.(neighborReader)
	if !ok || len(anchors) == 0 {
		return nil, nil
	}
	pool, err := ParsePool(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to capture cetus neighbors: %w", err)
	}
	var lower, upper *Tick
	for i := range anchors {
		t := &anchors[i]
		if t.Index <= pool.CurrentTickIndex && (lower == nil || t.Index > lower.Index) {
			lower = t
		}
		if t.Index > pool.CurrentTickIndex && (upper == nil || t.Index < upper.Index) {
			upper = t
		}
	}
	if lower == nil || upper == nil {
		return nil, nil
	}
	var meta struct {
		Manager struct {
			Ticks struct {
				ID string `json:"id"`
			} `json:"ticks"`
		} `json:"tick_manager"`
	}
	if err = json.Unmarshal(obj.Move.JSON, &meta); err != nil {
		return nil, fmt.Errorf("failed to capture cetus neighbors: %w", err)
	}
	handle, err := sui.ParseAddress(meta.Manager.Ticks.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to capture cetus neighbors: %w", err)
	}
	lo, hi := uint64(int64(lower.Index)+443636), uint64(int64(upper.Index)+443636)
	values, err := r.DynamicUint64ValuesAtCheckpoint(ctx, handle, cp, []uint64{lo, hi})
	if err != nil {
		return nil, fmt.Errorf("failed to capture cetus neighbors: %w", err)
	}
	if len(values) != 2 {
		return nil, fmt.Errorf("failed to capture cetus neighbors: values=invalid")
	}
	type link struct {
		None  bool            `json:"is_none"`
		Value json.RawMessage `json:"v"`
	}
	type node struct {
		Score json.RawMessage `json:"score"`
		Next  []link          `json:"nexts"`
		Prev  link            `json:"prev"`
	}
	nodes := make([]node, 2)
	ticks := make([]Tick, 2)
	for i, v := range values {
		if len(v) == 0 {
			return nil, nil
		}
		if err = json.Unmarshal(v, &nodes[i]); err != nil {
			return nil, fmt.Errorf("failed to capture cetus neighbors: %w", err)
		}
		ticks[i], err = parseTick(v)
		if err != nil {
			return nil, fmt.Errorf("failed to capture cetus neighbors: %w", err)
		}
	}
	if len(nodes[0].Next) == 0 || nodes[0].Next[0].None || nodes[1].Prev.None {
		return nil, nil
	}
	next, err := jsonUint64(nodes[0].Next[0].Value)
	if err != nil {
		return nil, fmt.Errorf("failed to capture cetus neighbors: %w", err)
	}
	prev, err := jsonUint64(nodes[1].Prev.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to capture cetus neighbors: %w", err)
	}
	if next != hi || prev != lo || ticks[0].Index != lower.Index || ticks[1].Index != upper.Index {
		return nil, nil
	}
	for i, expected := range []uint64{lo, hi} {
		score, err := jsonUint64(nodes[i].Score)
		if err != nil {
			return nil, fmt.Errorf("failed to capture cetus neighbors: %w", err)
		}
		if score != expected {
			return nil, fmt.Errorf("failed to capture cetus neighbors: score=mismatch")
		}
	}
	s := &LocalSnapshot{Pool: *pool, Ticks: ticks}
	// Only accept amounts completed inside the verified adjacent interval. If either
	// side reaches beyond it, obtain the complete snapshot instead.
	bid, err := s.Quote(params.Bid.AmountIn, params.Bid.A2B, true)
	if err != nil {
		return nil, nil
	}
	ask, err := s.Quote(params.Ask.AmountOut, params.Ask.A2B, false)
	if err != nil {
		return nil, nil
	}
	for _, q := range []QuoteResult{bid, ask} {
		if q.AfterSqrtPrice.Cmp(ticks[0].SqrtPrice) <= 0 || q.AfterSqrtPrice.Cmp(ticks[1].SqrtPrice) >= 0 {
			return nil, nil
		}
	}
	return s, nil
}
