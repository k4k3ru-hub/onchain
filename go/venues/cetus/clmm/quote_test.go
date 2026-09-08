package clmm

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type quoteTestSimulator struct {
	request onchainSui.SimulationRequest
	result  *onchainSui.SimulationResult
}

// TestQuoteExactInputReportsSimulationCommandDiagnostics verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestQuoteExactInputReportsSimulationCommandDiagnostics(t *testing.T) {
	quoter, sender, pool := quoteTestQuoter(t, &onchainSui.SimulationResult{})

	_, err := quoter.QuoteExactInput(context.Background(), QuoteExactInputParams{Sender: sender, Pool: pool, AmountIn: 100, A2B: true})
	if err == nil || !strings.Contains(err.Error(), "command_count=0 expected_command_count=1") {
		t.Fatalf("QuoteExactInput() error = %v", err)
	}
}

// TestQuoteExactInputReportsEmptyReturnValueDiagnostics verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestQuoteExactInputReportsEmptyReturnValueDiagnostics(t *testing.T) {
	quoter, sender, pool := quoteTestQuoter(t, &onchainSui.SimulationResult{
		CommandResults: []onchainSui.SimulationCommandResult{{MutatedByRef: []onchainSui.CommandOutput{{}}}},
		Events:         []onchainSui.SimulationEvent{{}},
	})

	_, err := quoter.QuoteExactInput(context.Background(), QuoteExactInputParams{Sender: sender, Pool: pool, AmountIn: 100, A2B: true})
	if err == nil || !strings.Contains(err.Error(), "return_value_count=0 mutated_by_ref_count=1 event_count=1") {
		t.Fatalf("QuoteExactInput() error = %v", err)
	}
}

// TestQuoteExactInputParsesSimulationEventResult verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestQuoteExactInputParsesSimulationEventResult(t *testing.T) {
	value := make([]byte, 49)
	binary.LittleEndian.PutUint64(value[0:8], 100)
	binary.LittleEndian.PutUint64(value[8:16], 99)
	binary.LittleEndian.PutUint64(value[16:24], 1)
	binary.LittleEndian.PutUint64(value[24:32], 100)
	value[32] = 5
	quoter, sender, pool := quoteTestQuoter(t, &onchainSui.SimulationResult{
		CommandResults: []onchainSui.SimulationCommandResult{{}},
		Events:         []onchainSui.SimulationEvent{{Type: "0x5::fetcher_script::CalculatedSwapResultEvent", BCS: value}},
	})

	result, err := quoter.QuoteExactInput(context.Background(), QuoteExactInputParams{Sender: sender, Pool: pool, AmountIn: 100, A2B: true})
	if err != nil {
		t.Fatalf("QuoteExactInput() error = %v", err)
	}
	if result.AmountIn != 100 || result.AmountOut != 99 || result.FeeAmount != 1 {
		t.Fatalf("QuoteExactInput() = %+v", result)
	}
}

// TestQuoteExactInputReportsEmptyReturnValueBCSDiagnostics verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestQuoteExactInputReportsEmptyReturnValueBCSDiagnostics(t *testing.T) {
	quoter, sender, pool := quoteTestQuoter(t, &onchainSui.SimulationResult{
		CommandResults: []onchainSui.SimulationCommandResult{{ReturnValues: []onchainSui.CommandOutput{{JSON: map[string]any{"amount_in": "100"}}}}},
	})

	_, err := quoter.QuoteExactInput(context.Background(), QuoteExactInputParams{Sender: sender, Pool: pool, AmountIn: 100, A2B: true})
	if err == nil || !strings.Contains(err.Error(), "return_value_bcs=empty return_value_json_present=true") {
		t.Fatalf("QuoteExactInput() error = %v", err)
	}
}

func quoteTestQuoter(t *testing.T, result *onchainSui.SimulationResult) (*Quoter, onchainSui.Address, Pool) {
	t.Helper()

	packageAddress, _ := onchainSui.ParseAddress("0x1")
	fetcherPackage, _ := onchainSui.ParseAddress("0x5")
	configAddress, _ := onchainSui.ParseAddress("0x2")
	clockAddress, _ := onchainSui.ParseAddress("0x6")
	poolAddress, _ := onchainSui.ParseAddress("0x3")
	sender, _ := onchainSui.ParseAddress("0x4")
	quoter, err := NewQuoter(Deployment{
		Package: packageAddress, PublishedAt: packageAddress, FetcherPackage: fetcherPackage,
		GlobalConfig: onchainSui.ObjectInput{Address: configAddress, Version: 1}, Clock: onchainSui.ObjectInput{Address: clockAddress, Version: 1},
		PoolModule: "pool", FetcherModule: "fetcher_script",
	}, &quoteTestSimulator{result: result})
	if err != nil {
		t.Fatalf("NewQuoter() error = %v", err)
	}
	return quoter, sender, Pool{Address: poolAddress, InitialVersion: 2, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC"}
}

func (s *quoteTestSimulator) SimulateTransaction(_ context.Context, request onchainSui.SimulationRequest) (*onchainSui.SimulationResult, error) {
	s.request = request
	return s.result, nil
}

// TestQuoteExactInput verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestQuoteExactInput(t *testing.T) {
	packageAddress, _ := onchainSui.ParseAddress("0x1")
	fetcherPackage, _ := onchainSui.ParseAddress("0x5")
	configAddress, _ := onchainSui.ParseAddress("0x2")
	clockAddress, _ := onchainSui.ParseAddress("0x6")
	poolAddress, _ := onchainSui.ParseAddress("0x3")
	sender, _ := onchainSui.ParseAddress("0x4")
	value := make([]byte, 49)
	binary.LittleEndian.PutUint64(value[0:8], 100)
	binary.LittleEndian.PutUint64(value[8:16], 99)
	binary.LittleEndian.PutUint64(value[16:24], 1)
	binary.LittleEndian.PutUint64(value[24:32], 100)
	value[32] = 5
	checkpoint := onchainSui.CheckpointSequenceNumber(123)
	simulator := &quoteTestSimulator{result: &onchainSui.SimulationResult{Checkpoint: checkpoint, CommandResults: []onchainSui.SimulationCommandResult{{ReturnValues: []onchainSui.CommandOutput{{BCS: value}}}}}}
	quoter, err := NewQuoter(Deployment{
		Package: packageAddress, PublishedAt: packageAddress, FetcherPackage: fetcherPackage,
		GlobalConfig: onchainSui.ObjectInput{Address: configAddress, Version: 1}, Clock: onchainSui.ObjectInput{Address: clockAddress, Version: 1},
		PoolModule: "pool", FetcherModule: "fetcher_script",
	}, simulator)
	if err != nil {
		t.Fatalf("NewQuoter() returned an unexpected error: %v", err)
	}
	result, err := quoter.QuoteExactInput(context.Background(), QuoteExactInputParams{Sender: sender, Pool: Pool{Address: poolAddress, InitialVersion: 2, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC"}, AmountIn: 100, A2B: true})
	if err != nil {
		t.Fatalf("QuoteExactInput() returned an unexpected error: %v", err)
	}
	if result.AmountOut != 99 || result.FeeAmount != 1 || result.Checkpoint != checkpoint || len(simulator.request.Transaction.Commands) != 1 || simulator.request.Transaction.Commands[0].MoveCall == nil || simulator.request.Transaction.Commands[0].MoveCall.Package != fetcherPackage || simulator.request.Transaction.Commands[0].MoveCall.Function != "calculate_swap_result" {
		t.Fatalf("QuoteExactInput() = %+v request=%+v", result, simulator.request)
	}
}

// TestQuoteExactOutput verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestQuoteExactOutput(t *testing.T) {
	packageAddress, _ := onchainSui.ParseAddress("0x1")
	fetcherPackage, _ := onchainSui.ParseAddress("0x5")
	configAddress, _ := onchainSui.ParseAddress("0x2")
	clockAddress, _ := onchainSui.ParseAddress("0x6")
	poolAddress, _ := onchainSui.ParseAddress("0x3")
	sender, _ := onchainSui.ParseAddress("0x4")
	value := make([]byte, 49)
	binary.LittleEndian.PutUint64(value[0:8], 101)
	binary.LittleEndian.PutUint64(value[8:16], 100)
	binary.LittleEndian.PutUint64(value[16:24], 1)
	binary.LittleEndian.PutUint64(value[24:32], 100)
	simulator := &quoteTestSimulator{result: &onchainSui.SimulationResult{CommandResults: []onchainSui.SimulationCommandResult{{ReturnValues: []onchainSui.CommandOutput{{BCS: value}}}}}}
	quoter, err := NewQuoter(Deployment{
		Package: packageAddress, PublishedAt: packageAddress, FetcherPackage: fetcherPackage,
		GlobalConfig: onchainSui.ObjectInput{Address: configAddress, Version: 1}, Clock: onchainSui.ObjectInput{Address: clockAddress, Version: 1},
		PoolModule: "pool", FetcherModule: "fetcher_script",
	}, simulator)
	if err != nil {
		t.Fatalf("NewQuoter() returned an unexpected error: %v", err)
	}
	result, err := quoter.QuoteExactOutput(context.Background(), QuoteExactOutputParams{Sender: sender, Pool: Pool{Address: poolAddress, InitialVersion: 2, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC"}, AmountOut: 100, A2B: false})
	if err != nil {
		t.Fatalf("QuoteExactOutput() returned an unexpected error: %v", err)
	}
	if result.AmountIn != 101 || len(simulator.request.Transaction.Inputs) != 4 || len(simulator.request.Transaction.Inputs[2].Pure) != 1 || simulator.request.Transaction.Inputs[2].Pure[0] != 0 {
		t.Fatalf("QuoteExactOutput() = %+v request=%+v", result, simulator.request)
	}
}

// TestQuotePairUsesOneSimulationCheckpoint verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestQuotePairUsesOneSimulationCheckpoint(t *testing.T) {
	value := make([]byte, 49)
	binary.LittleEndian.PutUint64(value[0:8], 100)
	binary.LittleEndian.PutUint64(value[8:16], 99)
	checkpoint := onchainSui.CheckpointSequenceNumber(123)
	quoter, sender, pool := quoteTestQuoter(t, &onchainSui.SimulationResult{Checkpoint: checkpoint, CommandResults: []onchainSui.SimulationCommandResult{
		{ReturnValues: []onchainSui.CommandOutput{{BCS: value}}},
		{ReturnValues: []onchainSui.CommandOutput{{BCS: value}}},
	}})

	result, err := quoter.QuotePair(context.Background(), QuotePairParams{
		Bid: QuoteExactInputParams{Sender: sender, Pool: pool, AmountIn: 100, A2B: true},
		Ask: QuoteExactOutputParams{Sender: sender, Pool: pool, AmountOut: 99, A2B: false},
	})
	if err != nil {
		t.Fatalf("QuotePair() error = %v", err)
	}
	if result.Checkpoint != checkpoint || result.Bid.Checkpoint != checkpoint || result.Ask.Checkpoint != checkpoint {
		t.Fatalf("QuotePair() = %+v", result)
	}
}
