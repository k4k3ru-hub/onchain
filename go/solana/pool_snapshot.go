package solana

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// PoolStateSnapshot describes detached account inputs, not an atomic cross-account ledger snapshot.
type PoolStateSnapshot struct {
	Protocol       string
	Token0, Token1 Address
	// RawPrice is token1 atomic units per token0 atomic unit, before fees.
	RawPrice     string
	PriceInputs  []PoolStateComponent
	Components   map[string]PoolStateComponent
	CoverageUnit string
	Coverage     []PoolCoverageSegment
}
type PoolCoverageSegment struct{ Lower, Upper int64 }
type PoolStateComponent struct {
	Slot       uint64
	Digest     string
	ReceivedAt time.Time
	Fields     map[string]string
}

// PoolComponent copies provenance for one retained account; Digest hashes account data, not a block.
//
// Version:
//   - 2026-09-12: Added.
func PoolComponent(account RetainedAccount, fields map[string]string) PoolStateComponent {
	var digest string
	if account.Account != nil {
		digest = fmt.Sprintf("%x", sha256.Sum256(account.Account.Data))
	}
	return PoolStateComponent{Slot: account.Slot.Uint64(), Digest: digest, ReceivedAt: account.ObservedAt, Fields: fields}
}
