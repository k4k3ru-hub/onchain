package solana

import "testing"

func TestQuoteArrayCounts(t *testing.T) {
	initial, maximum, err := NormalizeQuoteArrayCounts(0, 0)
	if err != nil || initial != 5 || maximum != 16 {
		t.Fatalf("defaults=%d/%d error=%v", initial, maximum, err)
	}
	for _, counts := range [][2]int{{-1, 16}, {5, -1}, {6, 5}, {5, 33}} {
		if _, _, err := NormalizeQuoteArrayCounts(counts[0], counts[1]); err == nil {
			t.Fatalf("accepted %v", counts)
		}
	}
}

func TestQuoteArraySelectionRecentersAndDeduplicates(t *testing.T) {
	a, b, c, d := Address{1}, Address{2}, Address{3}, Address{4}
	got := SelectQuoteArrays([]Address{b, c, d}, []Address{a, d, d}, 2)
	if len(got) != 3 || got[0] != b || got[1] != c || got[2] != d {
		t.Fatalf("selection=%v", got)
	}
}
