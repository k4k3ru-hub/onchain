package spot

import (
	"encoding/json"

	"github.com/k4k3ru-hub/onchain/go/internal/suievent"
)

// Field order is the deployed Move events::AssetSwap layout, verified 2026-09-11.
func decodeSwapBCS(data []byte) (json.RawMessage, error) {
	return suievent.JSON(data, []suievent.Field{
		{Name: "pool_id", Type: "address"},
		{Name: "a2b", Type: "bool"},
		{Name: "amount_in", Type: "u64"},
		{Name: "amount_out", Type: "u64"},
		{Name: "pool_coin_a_amount", Type: "u64"},
		{Name: "pool_coin_b_amount", Type: "u64"},
		{Name: "fee", Type: "u64"},
		{Name: "before_liquidity", Type: "u128"},
		{Name: "after_liquidity", Type: "u128"},
		{Name: "before_sqrt_price", Type: "u128"},
		{Name: "after_sqrt_price", Type: "u128"},
		{Name: "current_tick", Type: "u32"},
		{Name: "exceeded", Type: "bool"},
		{Name: "sequence_number", Type: "u128"},
	})
}
