package poolfee

import "testing"

// TestRateString verifies Pool swap fee behavior.
//
// Version:
//   - 2026-09-22: Added.
func TestRateString(t *testing.T) {
	for v, want := range map[uint32]string{0: "0", 1: "0.000001", 500: "0.0005", 3000: "0.003", 999999: "0.999999", 1000000: "1"} {
		got, err := RateString(v)
		if err != nil || got != want {
			t.Fatalf("ppm=%d got=%q err=%v", v, got, err)
		}
	}
	if _, err := RateString(1000001); err == nil {
		t.Fatal("accepted rate above 100%")
	}
}
