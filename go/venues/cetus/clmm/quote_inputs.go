package clmm

import (
	"strconv"

	"github.com/k4k3ru-hub/onchain/go/quotestate"
)

// Inputs returns detached retained-pool metadata without fetching data.
// Baseline remains the initialization checkpoint; Position follows the latest applied pool object.
// Retained fields may originate from multiple transactions; coverage is not an atomicity guarantee.
//
// Version:
//   - 2026-09-11: Report the verified tick interval, including uninitialized positions.
//   - 2026-09-10: Added.
func (s *QuoteSnapshot) Inputs() *quotestate.Inputs {
	if s == nil || s.cache.retained == nil {
		return nil
	}
	p := s.cache.retained
	pool := p.snapshot.Pool
	head := p.baseline
	baseline := quotestate.Position{Kind: "checkpoint", Sequence: head.SequenceNumber.Uint64(), Digest: head.Digest.String()}
	position := baseline
	if p.position != nil {
		position = *p.position
		if position.Index != nil {
			index := *position.Index
			position.Index = &index
		}
	}
	indices := make([]int64, 0, len(p.snapshot.Ticks))
	for _, tick := range p.snapshot.Ticks {
		indices = append(indices, int64(tick.Index))
	}
	coverage := quotestate.Segments(indices)
	if w := p.snapshot.window; w != nil {
		upper := int64(w.upper)
		if upper < 443636 {
			upper--
		}
		coverage = []quotestate.Segment{{Lower: int64(w.lower), Upper: upper}}
	}
	reason := ""
	if pool.Paused {
		reason = "pool_paused"
	}
	return &quotestate.Inputs{UnavailableReason: reason, Baseline: baseline, Position: position, ReceivedAt: s.ReceivedAt(), CoverageUnit: "tick", Coverage: coverage, Fields: map[string]string{
		"sqrt_price": quotestate.Integer(pool.CurrentSqrtPrice), "fraction_bits": "64", "liquidity": quotestate.Integer(pool.Liquidity), "tick": strconv.FormatInt(int64(pool.CurrentTickIndex), 10), "fee_ppm": strconv.FormatUint(uint64(pool.FeeRate), 10), "pool_version": strconv.FormatUint(p.version, 10), "pool_digest": p.digest.String(),
	}}
}
