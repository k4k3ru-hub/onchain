package clmm

import (
	"encoding/json"
	"testing"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

// TestParsePool verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestParsePool(t *testing.T) {
	address, _ := onchainSui.ParseAddress("0x123")
	object := &onchainSui.Object{Address: address, Version: 9, Move: &onchainSui.MoveObject{
		Type: "0x1::pool::Pool<0x2::sui::SUI, 0x3::usdc::USDC>",
		JSON: json.RawMessage(`{"coin_a":"100","coin_b":"200","current_sqrt_price":"18446744073709551616","current_tick_index":{"bits":4294967295},"liquidity":"300","fee_rate":"100","is_pause":false}`),
	}}
	pool, err := ParsePool(object)
	if err != nil {
		t.Fatalf("ParsePool() returned an unexpected error: %v", err)
	}
	if pool.CoinTypeA != "0x2::sui::SUI" || pool.CoinTypeB != "0x3::usdc::USDC" || pool.CurrentTickIndex != -1 || pool.FeeRate != 100 {
		t.Fatalf("ParsePool() = %+v", pool)
	}
}
