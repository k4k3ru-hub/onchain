package slipstream

import "testing"

// TestRetainedOracleUpdates verifies interpolation, growth, same-block writes and detached captures.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedOracleUpdates(t *testing.T) {
	o := retainedOracle{slots: []tickObservation{{100, 0, true}, {110, 100, true}}, index: 1, next: 4}
	frozen := o.clone()
	if err := o.write(120, -5); err != nil {
		t.Fatal(err)
	}
	if len(o.slots) != 4 || o.index != 2 || o.slots[2].cumulative != 50 {
		t.Fatalf("growth: %+v", o)
	}
	if err := o.write(120, 1000); err != nil {
		t.Fatal(err)
	}
	if o.index != 2 || frozen.slots[1].cumulative != 100 || len(frozen.slots) != 2 {
		t.Fatal("same block write or alias")
	}
	for _, tc := range []struct {
		ago  uint32
		want int64
		ok   bool
	}{{0, 90, true}, {5, 80, true}, {10, 70, true}, {15, 60, true}, {20, 50, true}, {25, 75, true}, {30, 100, true}, {35, 50, true}, {40, 0, true}, {41, 0, false}} {
		got, ok, err := o.observe(140, tc.ago, 2)
		if err != nil || ok != tc.ok || got != tc.want {
			t.Fatalf("ago=%d got=%d ok=%v err=%v", tc.ago, got, ok, err)
		}
	}
	for _, now := range []uint32{130, 140, 150, 160} {
		if err := o.write(now, 2); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok, err := o.observe(160, 41, 2); err != nil || ok {
		t.Fatalf("evicted history accepted: %v %v", ok, err)
	}
	if _, _, err := (retainedOracle{}).observe(160, 0, 2); err == nil {
		t.Fatal("missing data treated as short chain history")
	}
}

// TestRetainedOracleWrapAndFee verifies timestamp/cumulative wrapping and the connected dynamic-fee calculation.
//
// Version:
//   - 2026-09-09: Added.
func TestRetainedOracleWrapAndFee(t *testing.T) {
	o := retainedOracle{slots: []tickObservation{{^uint32(0) - 9, (1 << 55) - 5, true}}, next: 2}
	if err := o.write(10, 1); err != nil {
		t.Fatal(err)
	}
	got, ok, err := o.observe(10, 10, 1)
	if err != nil || !ok || got != -(1<<55)+5 {
		t.Fatalf("wrap got=%d ok=%v err=%v", got, ok, err)
	}
	config := dynamicFeeInputs{base: 3000, defaultCap: 50000, defaultScaling: 1000000, secondsAgo: 2}
	fee, err := o.fee(config, 10, 100)
	if err != nil || fee != 3099 {
		t.Fatalf("fee=%d err=%v", fee, err)
	}
	config.initialEnabled = true
	config.initial = 500
	fee, err = o.fee(config, 12, 100)
	if err != nil || fee != 500 {
		t.Fatalf("initial fee=%d err=%v", fee, err)
	}
}
