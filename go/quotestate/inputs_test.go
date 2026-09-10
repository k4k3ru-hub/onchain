package quotestate

import (
	"reflect"
	"testing"
)

// TestSegmentsPreserveGaps verifies retained input metadata and ordering.
//
// Version:
//   - 2026-09-10: Added.
func TestSegmentsPreserveGaps(t *testing.T) {
	input := []int64{3, -2, -1, 3, 6}
	before := append([]int64(nil), input...)
	got := Segments(input)
	if !reflect.DeepEqual(got, []Segment{{-2, -1}, {3, 3}, {6, 6}}) || !reflect.DeepEqual(input, before) {
		t.Fatal(got, input)
	}
}
