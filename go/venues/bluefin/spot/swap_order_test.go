package spot

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
		event := sui.LiveEvent{Checkpoint: 42, TransactionIndex: index, EventIndex: 7, Type: "0x1::events::AssetSwap", JSON: json.RawMessage(`{"pool_id":"0x9","a2b":true,"amount_in":"100","amount_out":"99","fee":"1","before_sqrt_price":"10","after_sqrt_price":"11"}`)}
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
	event := sui.Event{Checkpoint: 42, SequenceNumber: 7, Type: "0x1::events::AssetSwap", JSON: json.RawMessage(`{"pool_id":"0x9","a2b":true,"amount_in":"100","amount_out":"99","fee":"1","before_sqrt_price":"10","after_sqrt_price":"11"}`)}
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
