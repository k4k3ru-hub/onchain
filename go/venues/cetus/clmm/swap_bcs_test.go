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
	b, err := hex.DecodeString("01000000000000000000000000000000000000000000000000000000000000000a000000000000000000000000000000000000000000000000000000000000000b680000000000000069000000000000006a000000000000006b000000000000006c000000000000006d00000000000000090000000000000000000000010000000a0000000000000000000000010000007000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	event := sui.LiveEvent{Type: "0x1::pool::SwapEvent", Checkpoint: 42, TransactionIndex: 3, EventIndex: 7, BCS: b}
	got, err := ParseLiveSwapEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	original := event
	original.JSON = json.RawMessage(`{"atob":true,"pool":"0x000000000000000000000000000000000000000000000000000000000000000a","partner":"0x000000000000000000000000000000000000000000000000000000000000000b","amount_in":"104","amount_out":"105","ref_amount":"106","fee_amount":"107","vault_a_amount":"108","vault_b_amount":"109","before_sqrt_price":"79228162514264337593543950345","after_sqrt_price":"79228162514264337593543950346","steps":"112"}`)
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
	for _, offset := range []int{0} {
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
