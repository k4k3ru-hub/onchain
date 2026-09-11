package clmm

import (
	"encoding/json"

	"github.com/k4k3ru-hub/onchain/go/internal/suievent"
)

// Field order is the deployed Move trade::SwapEvent layout, verified 2026-09-11.
func decodeSwapBCS(data []byte) (json.RawMessage, error) {
	return suievent.JSON(data, []suievent.Field{
		{Name: "sender", Type: "address"},
		{Name: "pool_id", Type: "address"},
		{Name: "x_for_y", Type: "bool"},
		{Name: "amount_x", Type: "u64"},
		{Name: "amount_y", Type: "u64"},
		{Name: "sqrt_price_before", Type: "u128"},
		{Name: "sqrt_price_after", Type: "u128"},
		{Name: "liquidity", Type: "u128"},
		{Name: "tick_index", Type: "u32"},
		{Name: "fee_amount", Type: "u64"},
		{Name: "protocol_fee", Type: "u64"},
		{Name: "reserve_x", Type: "u64"},
		{Name: "reserve_y", Type: "u64"},
	})
}
