package dlmm

import (
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"github.com/k4k3ru-hub/onchain/go/solana/internal/programevents"
)

// HasSwapInstruction checks committed DLMM invocations before requesting transaction details.
// DLMM CPI event payloads are not present in ordinary logsSubscribe notifications.
//
// Version:
//   - 2026-09-12: Added.
func HasSwapInstruction(log *solana.Log, program solana.Address) (bool, error) {
	if program.IsZero() {
		program = MainnetProgramAddress()
	}
	events, err := programevents.Read(log, program)
	if err != nil {
		return false, err
	}
	for _, event := range events {
		switch event.Instruction {
		case "Swap", "Swap2", "SwapExactOut", "SwapExactOut2", "SwapWithPriceImpact", "SwapWithPriceImpact2":
			return true, nil
		}
	}
	return false, nil
}
