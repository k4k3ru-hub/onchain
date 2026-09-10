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
		event := sui.LiveEvent{Checkpoint: 42, TransactionIndex: index, EventIndex: 7, Type: "0x1::pool::SwapEvent", JSON: json.RawMessage(`{"pool":"0x9","recipient":"0xa","amount_a":"100","amount_b":"99","sqrt_price":"10","protocol_fee":"0","fee_amount":"1","a_to_b":true}`)}
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
	event := sui.Event{Checkpoint: 42, SequenceNumber: 7, Type: "0x1::pool::SwapEvent", JSON: json.RawMessage(`{"pool":"0x9","recipient":"0xa","amount_a":"100","amount_b":"99","sqrt_price":"10","protocol_fee":"0","fee_amount":"1","a_to_b":true}`)}
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
