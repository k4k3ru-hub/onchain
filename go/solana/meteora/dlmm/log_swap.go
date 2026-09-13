package dlmm

import (
	"errors"
	"fmt"

	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"github.com/k4k3ru-hub/onchain/go/solana/internal/programevents"
)

// HasSwapInstruction checks committed DLMM invocations before requesting transaction details.
// DLMM CPI event payloads are not present in ordinary logsSubscribe notifications.
//
// Returns:
//   - Whether the completed log prefix contains a committed Swap instruction.
//   - ErrExecutionLogsTruncated alongside that decision, or a fatal error with false.
//
// Version:
//   - 2026-09-13: Preserve committed instruction matches before execution log truncation.
//   - 2026-09-12: Added.
func HasSwapInstruction(log *solana.Log, program solana.Address) (bool, error) {
	if program.IsZero() {
		program = MainnetProgramAddress()
	}
	events, logErr := programevents.Read(log, program)
	if logErr != nil {
		logErr = fmt.Errorf("failed to check meteora swap instruction: %w", logErr)
		if !errors.Is(logErr, solana.ErrExecutionLogsTruncated) {
			return false, logErr
		}
	}
	for _, event := range events {
		switch event.Instruction {
		case "Swap", "Swap2", "SwapExactOut", "SwapExactOut2", "SwapWithPriceImpact", "SwapWithPriceImpact2":
			return true, logErr
		}
	}
	return false, logErr
}
