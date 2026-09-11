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
	b, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000009000000000000000000000000000000000000000000000000000000000000000a67000000000000006800000000000000040000000000000000000000010000006a0000006b000000070000000000000000000000010000006d000000000000006e000000000000000100")
	if err != nil {
		t.Fatal(err)
	}
	event := sui.LiveEvent{Type: "0x1::pool::SwapEvent", Checkpoint: 42, TransactionIndex: 3, EventIndex: 7, BCS: b}
	got, err := ParseLiveSwapEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	original := event
	original.JSON = json.RawMessage(`{"pool":"0x0000000000000000000000000000000000000000000000000000000000000009","recipient":"0x000000000000000000000000000000000000000000000000000000000000000a","amount_a":"103","amount_b":"104","liquidity":"79228162514264337593543950340","tick_current_index":"106","tick_pre_index":"107","sqrt_price":"79228162514264337593543950343","protocol_fee":"109","fee_amount":"110","a_to_b":true,"is_exact_in":false}`)
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
	for _, offset := range []int{136, 137} {
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
