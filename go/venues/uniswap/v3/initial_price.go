package v3

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"time"
)

// InitialPriceSnapshot describes the initialization read, not live quote readiness.
type InitialPriceSnapshot struct {
	BlockNumber  uint64
	BlockHash    common.Hash
	CapturedAt   time.Time
	SqrtPriceX96 *big.Int
	Tick         int32
}

// StoreInitialPrice stores a detached, block-pinned initialization price.
// It does not mark the cache ready for quotes or replace retained state.
//
// Version:
//   - 2026-09-16: Added.
func (c *StateCache) StoreInitialPrice(s InitialPriceSnapshot) error {
	if c == nil {
		return fmt.Errorf("failed to store initial pool price: state_cache=null")
	}
	if s.BlockHash == (common.Hash{}) || s.CapturedAt.IsZero() {
		return fmt.Errorf("failed to store initial pool price: block_metadata=invalid")
	}
	if s.SqrtPriceX96 == nil || s.SqrtPriceX96.Sign() <= 0 || s.SqrtPriceX96.BitLen() > 160 || s.Tick < -887272 || s.Tick > 887272 {
		return fmt.Errorf("failed to store initial pool price: slot0=invalid")
	}
	s.SqrtPriceX96 = new(big.Int).Set(s.SqrtPriceX96)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialPrice = &s
	return nil
}

// InitialPrice returns a detached initialization snapshot, which may be stale.
// Nil means no initialization price has been stored.
//
// Version:
//   - 2026-09-16: Added.
func (c *StateCache) InitialPrice() *InitialPriceSnapshot {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.initialPrice == nil {
		return nil
	}
	s := *c.initialPrice
	s.SqrtPriceX96 = new(big.Int).Set(s.SqrtPriceX96)
	return &s
}
