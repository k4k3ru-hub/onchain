package v3

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// TestRetainedReinitializationReasons distinguishes invalidation triggers without retaining unsafe inputs.
//
// Version:
//   - 2026-09-10: Added.
func TestRetainedReinitializationReasons(t *testing.T) {
	cases := []struct {
		reason string
		change func(*StateCache, *types.Log)
	}{
		{"removed log", func(_ *StateCache, l *types.Log) { l.Removed = true }},
		{"missing block hash", func(_ *StateCache, l *types.Log) { l.BlockHash = common.Hash{} }},
		{"missing snapshot", func(c *StateCache, _ *types.Log) { c.retained = nil }},
		{"baseline hash mismatch", func(c *StateCache, l *types.Log) { l.BlockNumber = c.retained.header.Number }},
		{"block order regression", func(c *StateCache, l *types.Log) { c.retainedBlock = l.BlockNumber + 1 }},
		{"stream block hash mismatch", func(c *StateCache, l *types.Log) { c.retainedHash = common.HexToHash("03") }},
		{"duplicate or out of order log", func(c *StateCache, l *types.Log) { c.retainedIndex = l.Index }},
		{"missing event topics", func(_ *StateCache, l *types.Log) { l.Topics = nil }},
		{"unsupported event", func(_ *StateCache, l *types.Log) { l.Topics[0] = common.HexToHash("04") }},
	}
	for _, tt := range cases {
		t.Run(tt.reason, func(t *testing.T) {
			c, _ := newTestCache(t)
			if _, err := c.QuotePair(context.Background(), big.NewInt(1000000), true); err != nil {
				t.Fatal(err)
			}
			log := testSwapLog(t, c.pool)
			log.BlockNumber, log.BlockHash, log.Index = 101, common.HexToHash("02"), 2
			c.retainedHasLog = true
			c.retainedBlock, c.retainedHash, c.retainedIndex = 101, log.BlockHash, 1
			tt.change(c, &log)
			err := c.applyRetainedLog(log)
			if err == nil || !strings.Contains(err.Error(), "reason="+`"`+tt.reason+`"`) {
				t.Fatalf("unexpected reason: %v", err)
			}
			if c.retained != nil {
				t.Fatal("invalid inputs remained available")
			}
		})
	}
}
