package clmm

import (
	"encoding/json"
	"testing"
)

// TestParseSwapEventAcceptsCurrentCetusSchemaWithoutSender verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestParseSwapEventAcceptsCurrentCetusSchemaWithoutSender(t *testing.T) {
	t.Parallel()

	swap, err := parseSwapEvent(
		"0x1::pool::SwapEvent",
		json.RawMessage(`{"pool":"0x9","atob":false,"amount_in":"100","amount_out":"99","fee_amount":"1","before_sqrt_price":"10","after_sqrt_price":"11"}`),
	)
	if err != nil {
		t.Fatalf("parseSwapEvent() error = %v", err)
	}
	if swap.A2B || swap.AmountIn != 100 || swap.AmountOut != 99 || !swap.Sender.IsZero() {
		t.Fatalf("parseSwapEvent() = %+v", swap)
	}
}

// TestParseSwapEventAcceptsLegacyA2BField verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestParseSwapEventAcceptsLegacyA2BField(t *testing.T) {
	t.Parallel()

	swap, err := parseSwapEvent(
		"0x1::pool::SwapEvent",
		json.RawMessage(`{"pool":"0x9","a2b":true,"amount_in":"100","amount_out":"99","fee_amount":"1","before_sqrt_price":"10","after_sqrt_price":"11"}`),
	)
	if err != nil {
		t.Fatalf("parseSwapEvent() error = %v", err)
	}
	if !swap.A2B {
		t.Fatalf("parseSwapEvent() A2B = %t, want true", swap.A2B)
	}
}

// TestParseSwapEventRejectsMissingDirection verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestParseSwapEventRejectsMissingDirection(t *testing.T) {
	t.Parallel()

	_, err := parseSwapEvent(
		"0x1::pool::SwapEvent",
		json.RawMessage(`{"pool":"0x9","amount_in":"100","amount_out":"99","fee_amount":"1","before_sqrt_price":"10","after_sqrt_price":"11"}`),
	)
	if err == nil {
		t.Fatal("parseSwapEvent() error = nil")
	}
}
