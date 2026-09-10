package slipstream

import (
	"github.com/k4k3ru-hub/onchain/go/quotestate"
	"strconv"
)

// Inputs returns detached metadata from the same frozen inputs used by QuotePair.
// Coverage describes loaded bitmap words; exact quantity coverage remains a local quote check.
//
// Version:
//   - 2026-09-11: Identify validated late-log replay corrections.
//   - 2026-09-10: Added.
func (s *QuoteSnapshot) Inputs() *quotestate.Inputs {
	if s == nil || s.cache.retained == nil {
		return nil
	}
	c := &s.cache
	p := s.cache.retained.pool
	if p == nil {
		return nil
	}
	baseline := quotestate.Position{Kind: "block", Sequence: p.header.Number, Digest: p.header.Hash.Hex()}
	position := baseline
	block, hash, index, hasLog := c.retained.block, c.retained.hash.Hex(), uint64(c.retained.index), c.retained.hasLog
	if hasLog {
		position = quotestate.Position{Kind: "log", Sequence: block, Digest: hash, Index: &index}
	}
	return &quotestate.Inputs{ReplayRevision: c.retained.replayRevision, Baseline: baseline, Position: position, ReceivedAt: s.ReceivedAt(), CoverageUnit: "bitmap_word", Coverage: quotestate.WordSegments(p.words), Fields: map[string]string{
		"sqrt_price": quotestate.Integer(p.price), "fraction_bits": "96", "liquidity": quotestate.Integer(p.liquidity), "tick": strconv.FormatInt(int64(p.tick), 10), "tick_spacing": strconv.FormatInt(int64(p.spacing), 10), "fee_model": "dynamic_oracle",
	}}
}
