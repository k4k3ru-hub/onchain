package programevents

import (
	"errors"
	"slices"
	"testing"

	solana "github.com/k4k3ru-hub/onchain/go/solana"
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

// TestReadTruncatedExecutionLogs checks the boundary between committed events and incomplete invocations.
//
// Version:
//   - 2026-09-17: Cover successful outer frames and unconfirmed nested ancestors.
//   - 2026-09-13: Added.
func TestReadTruncatedExecutionLogs(t *testing.T) {
	p := solana.Address{1}.String()
	parent := solana.Address{2}.String()
	invoke, success := "Program "+p+" invoke [1]", "Program "+p+" success"
	parentInvoke, parentSuccess := "Program "+parent+" invoke [1]", "Program "+parent+" success"
	childInvoke := "Program " + p + " invoke [2]"
	data := "Program data: AQ=="
	for _, tc := range []struct {
		name      string
		messages  []string
		indexes   []uint32
		failed    bool
		truncated bool
		fatal     bool
	}{
		{name: "completed_before_unfinished_call", messages: []string{invoke, data, success, parentInvoke, "Log truncated"}, indexes: []uint32{1}, truncated: true},
		{name: "completed_child_and_parent", messages: []string{parentInvoke, childInvoke, data, success, parentSuccess, "Log truncated"}, indexes: []uint32{2}, truncated: true},
		{name: "unfinished_parent", messages: []string{parentInvoke, childInvoke, data, success, "Log truncated"}, indexes: []uint32{2}, truncated: true},
		{name: "completed_child_then_sibling", messages: []string{parentInvoke, childInvoke, data, success, childInvoke, data, "Log truncated"}, indexes: []uint32{2}, truncated: true},
		{name: "unfinished_nested_parent", messages: []string{parentInvoke, "Program " + parent + " invoke [2]", "Program " + p + " invoke [3]", data, success, "Log truncated"}, truncated: true},
		{name: "failed_transaction_with_completed_child", messages: []string{parentInvoke, childInvoke, data, success, "Log truncated"}, failed: true},
		{name: "failed_parent", messages: []string{parentInvoke, childInvoke, data, success, "Program " + parent + " failed: error", "Log truncated"}, truncated: true},
		{name: "failed_transaction", messages: []string{invoke, data, success, "Log truncated"}, failed: true},
		{name: "multiple_completed_events", messages: []string{invoke, data, success, invoke, data, success, invoke, data, "Log truncated"}, indexes: []uint32{1, 4}, truncated: true},
		{name: "success_after_marker", messages: []string{invoke, data, "Log truncated", success}, truncated: true},
		{name: "first_marker_stops_parsing", messages: []string{invoke, data, success, "Log truncated", invoke, data, success, "Log truncated"}, indexes: []uint32{1}, truncated: true},
		{name: "malformed_tail_ignored", messages: []string{invoke, data, success, "Log truncated", "Program data: !!!", "Program " + p + " invoke [bad]"}, indexes: []uint32{1}, truncated: true},
		{name: "ordinary_marker_text", messages: []string{invoke, "Program log: diagnostic says Log truncated", data, success}, indexes: []uint32{2}},
		{name: "ordinary_invocation_text", messages: []string{invoke, "Program log: invoke [1]", "Program log: success", data, success}, indexes: []uint32{3}},
		{name: "missing_outer_invocation", messages: []string{childInvoke, data, success, "Log truncated"}, fatal: true},
		{name: "invalid_nested_depth", messages: []string{parentInvoke, invoke, data, success, parentSuccess, "Log truncated"}, fatal: true},
		{name: "missing_depth", messages: []string{"Program " + p + " invoke", data, success, "Log truncated"}, fatal: true},
		{name: "mismatched_close", messages: []string{invoke, data, parentSuccess, "Log truncated"}, fatal: true},
		{name: "malformed_data_before_marker", messages: []string{invoke, data, success, invoke, "Program data: !!!", "Log truncated"}, fatal: true},
		{name: "unfinished_without_marker", messages: []string{invoke, data, success, invoke, data}, fatal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, err := Read(&solana.Log{Messages: tc.messages, Failed: tc.failed}, solana.Address{1})
			if errors.Is(err, solana.ErrExecutionLogsTruncated) != tc.truncated || (err != nil) != (tc.truncated || tc.fatal) {
				t.Fatalf("unexpected error: %v", err)
			}
			var indexes []uint32
			for _, event := range events {
				indexes = append(indexes, event.Index)
				if !slices.Equal(event.Data, []byte{1}) {
					t.Fatalf("event data changed: %+v", event)
				}
			}
			if !slices.Equal(indexes, tc.indexes) {
				t.Fatalf("event indexes = %v, want %v", indexes, tc.indexes)
			}
		})
	}
}
