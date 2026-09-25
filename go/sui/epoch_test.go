package sui

import (
	"errors"
	"testing"
)

// TestCurrentEpoch verifies exact u64 decoding and rejects incomplete responses.
//
// Version:
//   - 2026-09-24: Added.
func TestCurrentEpoch(t *testing.T) {
	caller := &coinCaller{response: map[string]any{"epoch": map[string]any{"epochId": uint64(1232), "referenceGasPrice": "18446744073709551615"}}}
	client := composeRPCClient(RPCConfig{}, caller)
	epoch, err := client.CurrentEpoch(t.Context())
	if err != nil || epoch.ID != 1232 || epoch.ReferenceGasPrice != ^uint64(0) {
		t.Fatalf("epoch=%+v err=%v", epoch, err)
	}
	for _, response := range []any{map[string]any{}, map[string]any{"epoch": map[string]any{"referenceGasPrice": "1000"}}, map[string]any{"epoch": map[string]any{"epochId": 0, "referenceGasPrice": "0"}}, map[string]any{"epoch": map[string]any{"epochId": 1, "referenceGasPrice": "18446744073709551616"}}} {
		caller.response = response
		if _, err := client.CurrentEpoch(t.Context()); err == nil {
			t.Fatal("accepted invalid response")
		}
	}
	failure := errors.New("unavailable")
	caller.err = failure
	if _, err := client.CurrentEpoch(t.Context()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
