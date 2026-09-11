package clmm

import (
	"encoding/json"

	"github.com/k4k3ru-hub/onchain/go/internal/suievent"
)

// Field order is the deployed Move pool::SwapEvent layout, verified 2026-09-11.
func decodeSwapBCS(data []byte) (json.RawMessage, error) {
	return suievent.JSON(data, []suievent.Field{
		{Name: "atob", Type: "bool"},
		{Name: "pool", Type: "address"},
		{Name: "partner", Type: "address"},
		{Name: "amount_in", Type: "u64"},
		{Name: "amount_out", Type: "u64"},
		{Name: "ref_amount", Type: "u64"},
		{Name: "fee_amount", Type: "u64"},
		{Name: "vault_a_amount", Type: "u64"},
		{Name: "vault_b_amount", Type: "u64"},
		{Name: "before_sqrt_price", Type: "u128"},
		{Name: "after_sqrt_price", Type: "u128"},
		{Name: "steps", Type: "u64"},
	})
}
