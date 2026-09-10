package clmm

import (
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"strconv"
)

// Inputs returns detached retained-pool metadata without fetching data.
// Baseline remains the initialization checkpoint; Position follows the latest applied pool object.
// Retained fields may originate from multiple transactions; coverage is not an atomicity guarantee.
//
// Version:
//   - 2026-09-10: Added.
func (s *QuoteSnapshot) Inputs() *quotestate.Inputs {
	if s == nil || s.cache.retained == nil {
		return nil
	}
	p := s.cache.retained
	pool := p.pool
	head := s.cache.retainedHead
	baseline := quotestate.Position{Kind: "checkpoint", Sequence: head.SequenceNumber.Uint64(), Digest: head.Digest.String()}
	position := baseline
	if p.position != nil {
		position = *p.position
		if position.Index != nil {
			index := *position.Index
			position.Index = &index
		}
	}
	coverage := quotestate.WordSegments(p.words)
	return &quotestate.Inputs{Baseline: baseline, Position: position, ReceivedAt: s.ReceivedAt(), CoverageUnit: "bitmap_word", Coverage: coverage, Fields: map[string]string{
		"sqrt_price": quotestate.Integer(pool.SqrtPrice), "fraction_bits": "64", "liquidity": quotestate.Integer(pool.Liquidity), "tick": strconv.FormatInt(int64(pool.TickIndex), 10), "tick_spacing": strconv.FormatUint(uint64(pool.TickSpacing), 10), "fee_ppm": strconv.FormatUint(uint64(pool.FeeRate), 10), "pool_version": strconv.FormatUint(p.version, 10), "pool_digest": p.digest.String(),
	}}
}
