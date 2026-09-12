package programevents

import (
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"testing"
)

func TestCommittedProgramEvents(t *testing.T) {
	p := solana.Address{1}.String()
	other := solana.Address{2}.String()
	for _, tc := range []struct {
		name  string
		logs  []string
		count int
		bad   bool
	}{
		{"committed", []string{"Program " + p + " invoke [1]", "Program data: AQ==", "Program " + p + " success"}, 1, false},
		{"foreign", []string{"Program " + other + " invoke [1]", "Program data: AQ==", "Program " + other + " success"}, 0, false},
		{"rollback", []string{"Program " + other + " invoke [1]", "Program " + p + " invoke [2]", "Program data: AQ==", "Program " + p + " success", "Program " + other + " failed: error"}, 0, false},
		{"truncated", []string{"Program " + p + " invoke [1]", "Program data: AQ=="}, 0, true},
		{"invalid data", []string{"Program " + p + " invoke [1]", "Program data: !!!", "Program " + p + " success"}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, err := Read(&solana.Log{Messages: tc.logs}, solana.Address{1})
			if len(events) != tc.count || (err != nil) != tc.bad {
				t.Fatal(events, err)
			}
		})
	}
}
