package spot

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"testing"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type testSimulator struct {
	result  *onchainSui.SimulationResult
	request onchainSui.SimulationRequest
}

// SimulateTransaction returns the configured simulation result.
//
// Version:
//   - 2026-08-31: Added.
func (s *testSimulator) SimulateTransaction(_ context.Context, request onchainSui.SimulationRequest) (*onchainSui.SimulationResult, error) {
	s.request = request
	return s.result, nil
}

func TestPoolQuoteAndFlashSwap(t *testing.T) {
	poolAddress, _ := onchainSui.ParseAddress("0x9")
	object := &onchainSui.Object{Address: poolAddress, Version: 3, Move: &onchainSui.MoveObject{Type: "0x1::pool::Pool<0x2::sui::SUI, 0x3::usdc::USDC>", JSON: json.RawMessage(`{"coin_a":"100","coin_b":"200","current_sqrt_price":"10","liquidity":"300","fee_rate":"100","is_paused":false}`)}}
	pool, err := ParsePool(object)
	if err != nil {
		t.Fatalf("ParsePool() returned an unexpected error: %v", err)
	}
	value := make([]byte, 135)
	binary.LittleEndian.PutUint64(value[2:10], 100)
	binary.LittleEndian.PutUint64(value[18:26], 99)
	binary.LittleEndian.PutUint64(value[42:50], 1)
	value[74] = 11
	checkpoint := onchainSui.CheckpointSequenceNumber(123)
	simulator := &testSimulator{result: &onchainSui.SimulationResult{Checkpoint: checkpoint, CommandResults: []onchainSui.SimulationCommandResult{{ReturnValues: []onchainSui.CommandOutput{{BCS: value}}}}}}
	quoter, err := NewQuoter(testDeployment(), simulator)
	if err != nil {
		t.Fatalf("NewQuoter() returned an unexpected error: %v", err)
	}
	sender, _ := onchainSui.ParseAddress("0xa")
	quote, err := quoter.QuoteExactInput(context.Background(), QuoteExactInputParams{Sender: sender, Pool: *pool, AmountIn: 100, A2B: true, SqrtPriceLimit: big.NewInt(5)})
	if err != nil {
		t.Fatalf("QuoteExactInput() returned an unexpected error: %v", err)
	}
	if quote.AmountOut != 99 || quote.Checkpoint != checkpoint || simulator.request.Transaction.Commands[0].MoveCall.Function != "calculate_swap_results" {
		t.Fatalf("quote=%+v transaction=%+v", quote, simulator.request.Transaction)
	}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	flash, err := AppendFlashSwap(builder, testDeployment(), *pool, true, 100, big.NewInt(5))
	if err != nil {
		t.Fatalf("AppendFlashSwap() returned an unexpected error: %v", err)
	}
	if err := AppendRepayFlashSwap(builder, testDeployment(), *pool, flash, flash.BalanceA, flash.BalanceB); err != nil {
		t.Fatalf("AppendRepayFlashSwap() returned an unexpected error: %v", err)
	}
	tx, err := builder.Build()
	if err != nil || len(tx.Commands) != 2 {
		t.Fatalf("Build()=%+v, %v", tx, err)
	}
	builder = onchainSui.NewProgrammableTransactionBuilder()
	flash, err = AppendFlashSwap(builder, testDeployment(), *pool, true, 100, big.NewInt(5))
	if err != nil {
		t.Fatalf("AppendFlashSwap() returned an unexpected error: %v", err)
	}
	if _, err := AppendFlashSwapPayAmount(builder, testDeployment(), *pool, flash); err != nil {
		t.Fatalf("AppendFlashSwapPayAmount() returned an unexpected error: %v", err)
	}
	amount, err := onchainSui.AppendBalanceValue(builder, pool.CoinTypeB, flash.BalanceB)
	if err != nil {
		t.Fatalf("AppendBalanceValue() returned an unexpected error: %v", err)
	}
	if _, err := AppendSwap(builder, testDeployment(), *pool, SwapBalances{BalanceA: flash.BalanceA, BalanceB: flash.BalanceB}, false, true, amount, 0, big.NewInt(5)); err != nil {
		t.Fatalf("AppendSwap() returned an unexpected error: %v", err)
	}
	tx, err = builder.Build()
	if err != nil || len(tx.Commands) != 4 || tx.Commands[1].MoveCall.Function != "swap_pay_amount" || tx.Commands[3].MoveCall.Function != "swap" {
		t.Fatalf("Build()=%+v, %v", tx, err)
	}
}

func TestQuoterQuoteExactOutput(t *testing.T) {
	poolAddress, _ := onchainSui.ParseAddress("0x9")
	pool := Pool{Address: poolAddress, InitialVersion: 3, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC"}
	value := make([]byte, 135)
	binary.LittleEndian.PutUint64(value[2:10], 100)
	binary.LittleEndian.PutUint64(value[18:26], 102)
	binary.LittleEndian.PutUint64(value[42:50], 2)
	value[74] = 11
	simulator := &testSimulator{result: &onchainSui.SimulationResult{CommandResults: []onchainSui.SimulationCommandResult{{ReturnValues: []onchainSui.CommandOutput{{BCS: value}}}}}}
	quoter, err := NewQuoter(testDeployment(), simulator)
	if err != nil {
		t.Fatalf("NewQuoter() returned an unexpected error: %v", err)
	}
	sender, _ := onchainSui.ParseAddress("0xa")
	quote, err := quoter.QuoteExactOutput(context.Background(), QuoteExactOutputParams{Sender: sender, Pool: pool, AmountOut: 100, A2B: false, SqrtPriceLimit: big.NewInt(5)})
	if err != nil {
		t.Fatalf("QuoteExactOutput() returned an unexpected error: %v", err)
	}
	if quote.AmountIn != 102 || quote.AmountOut != 100 {
		t.Fatalf("QuoteExactOutput() = %+v", quote)
	}
	if len(simulator.request.Transaction.Inputs) != 5 || len(simulator.request.Transaction.Inputs[2].Pure) != 1 || simulator.request.Transaction.Inputs[2].Pure[0] != 0 {
		t.Fatalf("transaction inputs = %+v", simulator.request.Transaction.Inputs)
	}
}

func TestQuoterQuotePairUsesOneSimulationCheckpoint(t *testing.T) {
	poolAddress, _ := onchainSui.ParseAddress("0x9")
	sender, _ := onchainSui.ParseAddress("0xa")
	pool := Pool{Address: poolAddress, InitialVersion: 3, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC"}
	value := make([]byte, 135)
	binary.LittleEndian.PutUint64(value[2:10], 100)
	binary.LittleEndian.PutUint64(value[18:26], 99)
	checkpoint := onchainSui.CheckpointSequenceNumber(123)
	simulator := &testSimulator{result: &onchainSui.SimulationResult{Checkpoint: checkpoint, CommandResults: []onchainSui.SimulationCommandResult{
		{ReturnValues: []onchainSui.CommandOutput{{BCS: value}}},
		{ReturnValues: []onchainSui.CommandOutput{{BCS: value}}},
	}}}
	quoter, _ := NewQuoter(testDeployment(), simulator)
	result, err := quoter.QuotePair(context.Background(), QuotePairParams{
		Bid: QuoteExactInputParams{Sender: sender, Pool: pool, AmountIn: 100, A2B: true, SqrtPriceLimit: big.NewInt(5)},
		Ask: QuoteExactOutputParams{Sender: sender, Pool: pool, AmountOut: 100, A2B: false, SqrtPriceLimit: big.NewInt(5)},
	})
	if err != nil {
		t.Fatalf("QuotePair() error = %v", err)
	}
	if result.Checkpoint != checkpoint || result.Bid.Checkpoint != checkpoint || result.Ask.Checkpoint != checkpoint || len(simulator.request.Transaction.Commands) != 2 {
		t.Fatalf("QuotePair() = %+v commands=%d", result, len(simulator.request.Transaction.Commands))
	}
}

func TestParseSwapEventPreservesCheckpoint(t *testing.T) {
	checkpoint := onchainSui.CheckpointSequenceNumber(317_016_290)
	swap, err := ParseSwapEvent(onchainSui.Event{
		Checkpoint: checkpoint,
		Type:       "0x1::events::AssetSwap",
		JSON:       json.RawMessage(`{"pool_id":"0x9","a2b":true,"amount_in":"100","amount_out":"99","fee":"1","before_sqrt_price":"10","after_sqrt_price":"11"}`),
	})
	if err != nil {
		t.Fatalf("ParseSwapEvent() returned an unexpected error: %v", err)
	}
	if swap.Checkpoint != checkpoint {
		t.Fatalf("ParseSwapEvent().Checkpoint = %d, want %d", swap.Checkpoint, checkpoint)
	}
}

func testDeployment() Deployment {
	packageAddress, _ := onchainSui.ParseAddress("0x1")
	config, _ := onchainSui.ParseAddress("0x2")
	clock, _ := onchainSui.ParseAddress("0x6")
	return Deployment{Package: packageAddress, PublishedAt: packageAddress, GlobalConfig: onchainSui.ObjectInput{Address: config, Version: 1}, Clock: onchainSui.ObjectInput{Address: clock, Version: 1}, PoolModule: "pool", EventsModule: "events"}
}

// TestQuotePairInputProvenance verifies the pool input version independently of the observed head.
//
// Version:
//   - 2026-09-08: Added.
func TestQuotePairInputProvenance(t *testing.T) {
	pool := Pool{Address: testAddress("0x9"), InitialVersion: 1, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC"}
	value := make([]byte, 135)
	binary.LittleEndian.PutUint64(value[2:10], 100)
	binary.LittleEndian.PutUint64(value[18:26], 99)
	command := onchainSui.SimulationCommandResult{ReturnValues: []onchainSui.CommandOutput{{BCS: value}}}
	simulator := &testSimulator{result: &onchainSui.SimulationResult{Checkpoint: 123, InputObjects: []onchainSui.SimulationObjectReference{{Address: pool.Address, Version: 777, Digest: onchainSui.ObjectDigest{1}}}, CommandResults: []onchainSui.SimulationCommandResult{command, command}}}
	q, e := NewQuoter(testDeployment(), simulator)
	if e != nil {
		t.Fatal(e)
	}
	p := QuotePairParams{Bid: QuoteExactInputParams{Sender: testAddress("0xa"), Pool: pool, AmountIn: 100, A2B: true, SqrtPriceLimit: big.NewInt(5)}, Ask: QuoteExactOutputParams{Sender: testAddress("0xa"), Pool: pool, AmountOut: 100, A2B: false, SqrtPriceLimit: big.NewInt(10)}}
	r, e := q.QuotePair(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	if r.PoolVersion != 777 || r.PoolDigest != (onchainSui.ObjectDigest{1}) || r.Checkpoint != 123 || len(simulator.request.Transaction.Commands) != 2 {
		t.Fatalf("pair %+v", r)
	}
	simulator.result = nil
	if _, e = q.QuotePair(context.Background(), p); e == nil {
		t.Fatal("accepted missing simulation")
	}
}
