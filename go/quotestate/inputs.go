package quotestate

import (
	"math/big"
	"sort"
	"time"
)

// Position identifies an end-of-block/checkpoint baseline or an applied log/transaction.
// Index is optional; nil never means index zero. Kind defines the index scope.
type Position struct {
	Kind     string // block, log, checkpoint, transaction
	Sequence uint64
	Digest   string
	Index    *uint64
}

type Segment struct{ Lower, Upper int64 }

// Inputs describes frozen receiver-side inputs, not a claim that all components
// were fetched at Position. Baseline identifies initialization; Fields describes
// the latest pool object, and coverage lists actually retained bitmap words/ticks.
type Inputs struct {
	// ReplayRevision advances only after validated late-log replay within a baseline.
	ReplayRevision     uint64
	UnavailableReason  string // Empty means no known pool-level prohibition; quantity checks still apply.
	Baseline, Position Position
	ReceivedAt         time.Time
	Fields             map[string]string
	CoverageUnit       string
	Coverage           []Segment
}

// Segments returns sorted disjoint runs of actually loaded indices.
//
// Version:
//   - 2026-09-10: Added.
func Segments(indices []int64) []Segment {
	indices = append([]int64(nil), indices...)
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })
	var result []Segment
	for _, index := range indices {
		if len(result) == 0 || (index > result[len(result)-1].Upper && index-result[len(result)-1].Upper != 1) {
			result = append(result, Segment{index, index})
			continue
		}
		if index > result[len(result)-1].Upper {
			result[len(result)-1].Upper = index
		}
	}
	return result
}

// WordSegments describes loaded bitmap words, including known zero words.
//
// Version:
//   - 2026-09-10: Added.
func WordSegments(words map[int32]*big.Int) []Segment {
	indices := make([]int64, 0, len(words))
	for index, value := range words {
		if value != nil {
			indices = append(indices, int64(index))
		}
	}
	return Segments(indices)
}

// Integer returns an optional integer field without exposing mutable arithmetic inputs.
//
// Version:
//   - 2026-09-10: Added.
func Integer(value *big.Int) string {
	if value == nil {
		return ""
	}
	return value.String()
}
