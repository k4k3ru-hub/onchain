package clmm

import (
	"encoding/json"

	"github.com/k4k3ru-hub/onchain/go/internal/suievent"
)

// Field order is the deployed Move pool::SwapEvent layout, verified 2026-09-11.
func decodeSwapBCS(data []byte) (json.RawMessage, error) {
	return suievent.JSON(data, []suievent.Field{
		{Name: "pool", Type: "address"},
		{Name: "recipient", Type: "address"},
		{Name: "amount_a", Type: "u64"},
		{Name: "amount_b", Type: "u64"},
		{Name: "liquidity", Type: "u128"},
		{Name: "tick_current_index", Type: "u32"},
		{Name: "tick_pre_index", Type: "u32"},
		{Name: "sqrt_price", Type: "u128"},
		{Name: "protocol_fee", Type: "u64"},
		{Name: "fee_amount", Type: "u64"},
		{Name: "a_to_b", Type: "bool"},
		{Name: "is_exact_in", Type: "bool"},
	})
}
