package clmm

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	sui "github.com/k4k3ru-hub/onchain/go/sui"
)

// TestLiveSwapBCS verifies complete BCS decoding, JSON compatibility and malformed input rejection.
//
// Version:
//   - 2026-09-11: Added.
func TestLiveSwapBCS(t *testing.T) {
	b, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000009000000000000000000000000000000000000000000000000000000000000000a01680000000000000069000000000000000500000000000000000000000100000006000000000000000000000001000000070000000000000000000000010000006d0000006e000000000000006f0000000000000070000000000000007100000000000000")
	if err != nil {
		t.Fatal(err)
	}
	event := sui.LiveEvent{Type: "0x1::trade::SwapEvent", Checkpoint: 42, TransactionIndex: 3, EventIndex: 7, BCS: b}
	got, err := ParseLiveSwapEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	original := event
	original.JSON = json.RawMessage(`{"sender":"0x0000000000000000000000000000000000000000000000000000000000000009","pool_id":"0x000000000000000000000000000000000000000000000000000000000000000a","x_for_y":true,"amount_x":"104","amount_y":"105","sqrt_price_before":"79228162514264337593543950341","sqrt_price_after":"79228162514264337593543950342","liquidity":"79228162514264337593543950343","tick_index":"109","fee_amount":"110","protocol_fee":"111","reserve_x":"112","reserve_y":"113"}`)
	original.BCS = nil
	want, err := ParseLiveSwapEvent(original)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BCS differs from JSON: got=%+v want=%+v", got, want)
	}
	for i := 0; i < len(b); i++ {
		bad := event
		bad.BCS = b[:i]
		if _, err := ParseLiveSwapEvent(bad); err == nil {
			t.Fatalf("accepted truncated body length=%d", i)
		}
	}
	bad := event
	bad.BCS = append(append([]byte(nil), b...), 0)
	if _, err := ParseLiveSwapEvent(bad); err == nil {
		t.Fatal("accepted trailing bytes")
	}
	for _, offset := range []int{64} {
		bad = event
		bad.BCS = append([]byte(nil), b...)
		bad.BCS[offset] = 2
		if _, err := ParseLiveSwapEvent(bad); err == nil {
			t.Fatal("accepted invalid bool")
		}
	}
	bad = event
	bad.JSON = json.RawMessage(`{`)
	if _, err := ParseLiveSwapEvent(bad); err == nil {
		t.Fatal("hid malformed JSON behind BCS")
	}
	bad = event
	bad.Type = "0x1::other::Event"
	if _, err := ParseLiveSwapEvent(bad); err == nil {
		t.Fatal("accepted wrong event type")
	}
}
