package dlmm

import (
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"testing"
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
