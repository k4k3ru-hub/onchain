package solana

import (
	"context"
	"fmt"
	"time"
)

type Block struct {
	Slot         Slot
	Hash         Hash
	PreviousHash Hash
	ParentSlot   Slot
	Height       *BlockHeight
	Timestamp    *time.Time
	Signatures   []Signature
}

type blockProvider interface {
	getBlock(context.Context, Slot, Commitment) (*Block, error)
}

// Block returns a confirmed Solana block by slot.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - slot: block slot.
//
// Returns:
//   - Solana block.
//   - Retrieval error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) Block(ctx context.Context, slot Slot) (*Block, error) {
	if c == nil || c.blockProvider == nil {
		return nil, fmt.Errorf("failed to get solana block: block_provider=null")
	}
	if err := slot.Validate(); err != nil {
		return nil, fmt.Errorf("failed to get solana block: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	block, err := c.blockProvider.getBlock(ctx, slot, c.config.Commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to get solana block: %w", err)
	}
	if block == nil {
		return nil, fmt.Errorf("failed to get solana block: block=null")
	}
	return block, nil
}
