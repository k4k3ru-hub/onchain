package solana

import (
	"context"
	"fmt"
	"strconv"
)

type Slot uint64

type slotProvider interface {
	getSlot(ctx context.Context, commitment Commitment) (Slot, error)
}

// Uint64 returns the numeric Solana slot.
//
// Returns:
//   - Numeric slot.
//
// Version:
//   - 2026-08-22: Added.
func (s Slot) Uint64() uint64 {
	return uint64(s)
}

// String returns the decimal Solana slot.
//
// Returns:
//   - Decimal slot.
//
// Version:
//   - 2026-08-22: Added.
func (s Slot) String() string {
	return strconv.FormatUint(s.Uint64(), 10)
}

// Validate validates a Solana slot.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-22: Added.
func (s Slot) Validate() error {
	if s == 0 {
		return fmt.Errorf("failed to validate solana slot: slot=empty")
	}
	return nil
}

// Slot returns the current Solana slot at the configured commitment.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//
// Returns:
//   - Current Solana slot.
//   - Retrieval error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) Slot(ctx context.Context) (Slot, error) {
	if c == nil {
		return 0, fmt.Errorf("failed to get solana slot: rpc_client=null")
	}
	if c.slotProvider == nil {
		return 0, fmt.Errorf("failed to get solana slot: slot_provider=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	slot, err := c.slotProvider.getSlot(ctx, c.config.Commitment)
	if err != nil {
		return 0, fmt.Errorf("failed to get solana slot: %w", err)
	}
	if err := slot.Validate(); err != nil {
		return 0, fmt.Errorf("failed to get solana slot: %w", err)
	}
	return slot, nil
}
