package slipstream

import "testing"

// TestDynamicFeeBranches checks contract fee branches and integer rounding.
//
// Version:
//   - 2026-09-09: Added.
func TestDynamicFeeBranches(t *testing.T) {
	base := dynamicFeeInputs{base: 3000, spacingFee: 500, defaultCap: 50000, defaultScaling: 1_000_000, secondsAgo: 600, cardinality: 300, tick: 100, cumulativePast: 0, cumulativeNow: 6000, oracleAvailable: true, timestamp: 1000, lastObservation: 1000}
	for _, tc := range []struct {
		name   string
		modify func(*dynamicFeeInputs)
		want   uint32
	}{
		{"dynamic", func(s *dynamicFeeInputs) {}, 3090},
		{"zero marker", func(s *dynamicFeeInputs) { s.base = 420 }, 0},
		{"spacing fallback", func(s *dynamicFeeInputs) { s.base = 0 }, 590},
		{"cap", func(s *dynamicFeeInputs) { s.defaultCap = 3001 }, 3001},
		{"pool scaling", func(s *dynamicFeeInputs) { s.scaling = 2_000_000; s.cap = 50000 }, 3180},
		{"negative truncation", func(s *dynamicFeeInputs) { s.cumulativeNow = -601 }, 3101},
		{"unavailable oracle", func(s *dynamicFeeInputs) { s.oracleAvailable = false }, 3000},
		{"insufficient cardinality", func(s *dynamicFeeInputs) { s.cardinality = 299 }, 3000},
		{"discount rounds up", func(s *dynamicFeeInputs) { s.discount = 1 }, 3089},
		{"initial base ignores discount", func(s *dynamicFeeInputs) { s.initialEnabled = true; s.lastObservation = 999; s.discount = 500000 }, 3000},
		{"initial override", func(s *dynamicFeeInputs) { s.initialEnabled = true; s.lastObservation = 999; s.initial = 100 }, 100},
		{"initial zero", func(s *dynamicFeeInputs) { s.initialEnabled = true; s.lastObservation = 999; s.initial = 420 }, 0},
		{"current block dynamic", func(s *dynamicFeeInputs) { s.initialEnabled = true; s.initial = 100 }, 3090},
		{"large scaling capped", func(s *dynamicFeeInputs) { s.defaultScaling = 1e18 }, 50000},
		{"wrapped cumulative", func(s *dynamicFeeInputs) { s.cumulativePast = (1 << 55) - 1; s.cumulativeNow = -(1 << 55) + 599 }, 3099},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.modify(&s)
			got, err := calculateDynamicFee(s)
			if err != nil || got != tc.want {
				t.Fatalf("fee=%d want=%d err=%v", got, tc.want, err)
			}
		})
	}
	for _, mutate := range []func(*dynamicFeeInputs){func(s *dynamicFeeInputs) { s.secondsAgo = 0 }, func(s *dynamicFeeInputs) { s.discount = 500001 }, func(s *dynamicFeeInputs) { s.cumulativeNow = 1 << 55 }} {
		s := base
		mutate(&s)
		if _, err := calculateDynamicFee(s); err == nil {
			t.Fatal("invalid inputs accepted")
		}
	}
}
