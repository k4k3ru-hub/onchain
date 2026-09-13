package dlmm

import (
	"errors"
	"testing"

	solana "github.com/k4k3ru-hub/onchain/go/solana"
)

func TestHasSwapInstruction(t *testing.T) {
	p := MainnetProgramAddress().String()
	for _, tc := range []struct {
		name string
		want bool
	}{{"Swap2", true}, {"SwapExactOut", true}, {"AddLiquidity", false}} {
		log := &solana.Log{Messages: []string{"Program " + p + " invoke [1]", "Program log: Instruction: " + tc.name, "Program " + p + " success"}}
		found, err := HasSwapInstruction(log, solana.Address{})
		if err != nil || found != tc.want {
			t.Fatal(found, err)
		}
		log.Failed = true
		found, err = HasSwapInstruction(log, solana.Address{})
		if err != nil || found {
			t.Fatal(found, err)
		}
	}
}

// TestHasSwapInstructionTruncatedPrefix checks that only completed Swap invocations admit transaction resolution.
//
// Version:
//   - 2026-09-13: Added.
func TestHasSwapInstructionTruncatedPrefix(t *testing.T) {
	p := MainnetProgramAddress().String()
	parent := solana.Address{2}.String()
	for _, tc := range []struct {
		name     string
		messages []string
		want     bool
	}{
		{"completed", []string{"Program " + p + " invoke [1]", "Program log: Instruction: Swap2", "Program " + p + " success", "Log truncated"}, true},
		{"completed_parent", []string{"Program " + parent + " invoke [1]", "Program " + p + " invoke [2]", "Program log: Instruction: Swap2", "Program " + p + " success", "Program " + parent + " success", "Log truncated"}, true},
		{"unfinished_parent", []string{"Program " + parent + " invoke [1]", "Program " + p + " invoke [2]", "Program log: Instruction: Swap2", "Program " + p + " success", "Log truncated"}, false},
		{"failed_parent", []string{"Program " + parent + " invoke [1]", "Program " + p + " invoke [2]", "Program log: Instruction: Swap2", "Program " + p + " success", "Program " + parent + " failed: error", "Log truncated"}, false},
		{"unfinished", []string{"Program " + p + " invoke [1]", "Program log: Instruction: Swap2", "Log truncated", "Program " + p + " success"}, false},
		{"unrelated_instruction", []string{"Program " + p + " invoke [1]", "Program log: Instruction: AddLiquidity", "Program " + p + " success", "Log truncated"}, false},
		{"foreign_program", []string{"Program " + parent + " invoke [1]", "Program log: Instruction: Swap2", "Program " + parent + " success", "Log truncated"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := &solana.Log{Messages: tc.messages}
			found, err := HasSwapInstruction(log, solana.Address{})
			if found != tc.want || !errors.Is(err, solana.ErrExecutionLogsTruncated) {
				t.Fatalf("found = %t, error = %v", found, err)
			}
			log.Failed = true
			if found, err = HasSwapInstruction(log, solana.Address{}); found || err != nil {
				t.Fatalf("failed transaction admitted: found = %t, error = %v", found, err)
			}
		})
	}
}
