package slipstream

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

// Diagnostics returns an independent cumulative snapshot of bounded-label cache counters.
// Notification block relationships refer to the last accepted snapshot; rejection
// relationships refer to the quote's captured block. Rejection labels may overlap.
//
// Version:
//   - 2026-09-08: Added.
func (c *StateCache) Diagnostics() map[string]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]uint64, len(c.diagnostics))
	for k, v := range c.diagnostics {
		result[k] = v
	}
	return result
}

// countDiagnostic requires mu to be held; labels must be internally bounded.
func (c *StateCache) countDiagnostic(label string) {
	if c.diagnostics == nil {
		c.diagnostics = make(map[string]uint64)
	}
	c.diagnostics[label]++
}
func (c *StateCache) recordNotification(log types.Log) {
	source := "pool"
	if log.Address == c.factory {
		source = "factory"
	}
	relation := "same_block"
	switch {
	case log.Removed:
		relation = "removed"
	case log.BlockHash == (common.Hash{}):
		relation = "missing_hash"
	case c.covered.Hash == (common.Hash{}):
		relation = "no_snapshot"
	case log.BlockNumber > c.covered.Number:
		relation = "newer_block"
	case log.BlockNumber < c.covered.Number:
		relation = "older_block"
	case log.BlockHash != c.covered.Hash:
		relation = "hash_mismatch"
	}
	c.countDiagnostic("notification." + source + "." + relation)
}
func (c *StateCache) recordInvalidation(n *quoteNotifications, delta uint64, header evm.BlockHeader) {
	c.countDiagnostic("invalidation.total")
	if delta != n.count {
		c.countDiagnostic("invalidation.lifecycle")
	}
	if n.count == 0 {
		return
	}
	if n.removed {
		c.countDiagnostic("invalidation.removed")
	}
	if n.missingHash {
		c.countDiagnostic("invalidation.missing_hash")
	}
	if n.maxBlock > header.Number {
		c.countDiagnostic("invalidation.newer_block")
	}
	if n.minBlock < header.Number {
		c.countDiagnostic("invalidation.older_block")
	}
	if n.minBlock != n.maxBlock {
		c.countDiagnostic("invalidation.mixed_blocks")
	}
	if n.minBlock == header.Number && n.maxBlock == header.Number && !n.missingHash && (n.block.Hash != header.Hash || (n.unsafe && !n.removed)) {
		c.countDiagnostic("invalidation.hash_mismatch")
	}
}
