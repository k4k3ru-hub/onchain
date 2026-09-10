package clmm

import (
	"encoding/json"
	sui "github.com/k4k3ru-hub/onchain/go/sui"
	"testing"
)

// TestSwapTransactionOrder preserves zero indices and distinguishes absent history order.
//
// Version:
//   - 2026-09-10: Added.
func TestSwapTransactionOrder(t *testing.T) {
	for _, index := range []uint64{0, 3} {
		event := sui.LiveEvent{Checkpoint: 42, TransactionIndex: index, EventIndex: 7, Type: "0x1::trade::SwapEvent", JSON: json.RawMessage(`{"pool_id":"0x9","sender":"0xa","x_for_y":true,"amount_x":"100","amount_y":"99","fee_amount":"1","protocol_fee":"0","sqrt_price_before":"10","sqrt_price_after":"11"}`)}
		swap, err := ParseLiveSwapEvent(event)
		if err != nil {
			t.Fatal(err)
		}
		if swap.TransactionIndex == nil || *swap.TransactionIndex != index || swap.EventIndex != 7 {
			t.Fatalf("order lost: %+v", swap)
		}
		event.TransactionIndex = 99
		if *swap.TransactionIndex != index {
			t.Fatal("input alias")
		}
	}
	event := sui.Event{Checkpoint: 42, SequenceNumber: 7, Type: "0x1::trade::SwapEvent", JSON: json.RawMessage(`{"pool_id":"0x9","sender":"0xa","x_for_y":true,"amount_x":"100","amount_y":"99","fee_amount":"1","protocol_fee":"0","sqrt_price_before":"10","sqrt_price_after":"11"}`)}
	swap, err := ParseSwapEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if swap.TransactionIndex != nil {
		t.Fatal("history fabricated zero index")
	}
	index := uint64(3)
	event.TransactionIndex = &index
	swap, err = ParseSwapEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	index = 99
	if swap.TransactionIndex == nil || *swap.TransactionIndex != 3 {
		t.Fatal("ordered history lost index or alias")
	}
}
