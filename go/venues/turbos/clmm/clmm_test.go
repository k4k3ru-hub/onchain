package clmm

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

func TestPoolQuoteAndSwapPTB(t *testing.T) {
	poolAddress, _ := onchainSui.ParseAddress("0x9")
	object := &onchainSui.Object{Address: poolAddress, Version: 3, Move: &onchainSui.MoveObject{Type: "0x1::pool::Pool<0x2::sui::SUI, 0x3::usdc::USDC, 0x1::fee::FEE>", JSON: json.RawMessage(`{"coin_a":"100","coin_b":"200","sqrt_price":"10","liquidity":"300","tick_current_index":{"bits":1},"tick_spacing":2,"fee":100,"unlocked":true}`)}}
	pool, err := ParsePool(object)
	if err != nil {
		t.Fatalf("ParsePool() returned an unexpected error: %v", err)
	}
	event := make([]byte, 138)
	binary.LittleEndian.PutUint64(event[64:72], 100)
	binary.LittleEndian.PutUint64(event[72:80], 99)
	binary.LittleEndian.PutUint64(event[128:136], 1)
	event[104] = 11
	checkpoint := onchainSui.CheckpointSequenceNumber(123)
	simulator := &testSimulator{result: &onchainSui.SimulationResult{Checkpoint: checkpoint, Events: []onchainSui.SimulationEvent{{Type: "0x1::pool::SwapEvent", BCS: event}}}}
	quoter, err := NewQuoter(testDeployment(), simulator)
	if err != nil {
		t.Fatalf("NewQuoter() returned an unexpected error: %v", err)
	}
	sender, _ := onchainSui.ParseAddress("0xa")
	quote, err := quoter.QuoteExactInput(context.Background(), QuoteExactInputParams{Sender: sender, Pool: *pool, AmountIn: big.NewInt(100), A2B: true, SqrtPriceLimit: big.NewInt(5)})
	if err != nil || quote.AmountOut.Uint64() != 99 || quote.Checkpoint != checkpoint {
		t.Fatalf("QuoteExactInput()=%+v, %v", quote, err)
	}
	quote, err = quoter.QuoteExactOutput(context.Background(), QuoteExactOutputParams{Sender: sender, Pool: *pool, AmountOut: big.NewInt(99), A2B: true, SqrtPriceLimit: big.NewInt(5)})
	if err != nil || quote.AmountIn.Uint64() != 100 {
		t.Fatalf("QuoteExactOutput()=%+v, %v", quote, err)
	}
	if len(simulator.request.Transaction.Inputs) < 4 || len(simulator.request.Transaction.Inputs[3].Pure) != 1 || simulator.request.Transaction.Inputs[3].Pure[0] != 0 {
		t.Fatalf("exact_output_transaction=%+v", simulator.request.Transaction)
	}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	packageAddress, _ := onchainSui.ParseAddress("0x2")
	coin, err := builder.MoveCall(onchainSui.MoveCall{Package: packageAddress, Module: "coin", Function: "zero", TypeArguments: []string{pool.CoinTypeA}})
	if err != nil {
		t.Fatalf("MoveCall() returned an unexpected error: %v", err)
	}
	recipient, _ := onchainSui.ParseAddress("0xb")
	swapArguments, err := AppendSwapExactInput(builder, testDeployment(), SwapExactInputParams{Pool: *pool, InputCoin: coin, AmountIn: 100, MinimumOut: 90, SqrtPriceLimit: big.NewInt(5), A2B: true, Recipient: recipient, DeadlineMS: 1000})
	if err != nil {
		t.Fatalf("AppendSwapExactInput() returned an unexpected error: %v", err)
	}
	tx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned an unexpected error: %v", err)
	}
	if len(tx.Commands) != 3 || tx.Commands[1].Kind != onchainSui.CommandKindMakeMoveVec || tx.Commands[2].MoveCall.Function != "swap_a_b_with_return_" {
		t.Fatalf("transaction=%+v", tx)
	}
	if swapArguments.CoinA.Subresult == nil || *swapArguments.CoinA.Subresult != 1 || swapArguments.CoinB.Subresult == nil || *swapArguments.CoinB.Subresult != 0 {
		t.Fatalf("AppendSwapExactInput()=%+v", swapArguments)
	}
}

// TestQuotePairUsesOneSimulationCheckpoint verifies pair composition and exact input provenance.
//
// Version:
//   - 2026-09-08: Verify input version and digest.
func TestQuotePairUsesOneSimulationCheckpoint(t *testing.T) {
	poolAddress, _ := onchainSui.ParseAddress("0x9")
	sender, _ := onchainSui.ParseAddress("0xa")
	pool := Pool{Address: poolAddress, InitialVersion: 3, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC", FeeType: "0x1::fee::FEE"}
	event := make([]byte, 138)
	binary.LittleEndian.PutUint64(event[64:72], 100)
	binary.LittleEndian.PutUint64(event[72:80], 99)
	checkpoint := onchainSui.CheckpointSequenceNumber(123)
	simulator := &testSimulator{result: &onchainSui.SimulationResult{InputObjects: []onchainSui.SimulationObjectReference{{Address: poolAddress, Version: 777, Digest: onchainSui.ObjectDigest{1}}}, Checkpoint: checkpoint, Events: []onchainSui.SimulationEvent{
		{Type: "0x1::pool::SwapEvent", BCS: event}, {Type: "0x1::pool::SwapEvent", BCS: event},
	}}}
	quoter, _ := NewQuoter(testDeployment(), simulator)
	result, err := quoter.QuotePair(context.Background(), QuotePairParams{
		Bid: QuoteExactInputParams{Sender: sender, Pool: pool, AmountIn: big.NewInt(100), A2B: true, SqrtPriceLimit: big.NewInt(5)},
		Ask: QuoteExactOutputParams{Sender: sender, Pool: pool, AmountOut: big.NewInt(99), A2B: false, SqrtPriceLimit: big.NewInt(5)},
	})
	if err != nil {
		t.Fatalf("QuotePair() error = %v", err)
	}
	if result.PoolVersion != 777 || result.PoolDigest != (onchainSui.ObjectDigest{1}) || result.Checkpoint != checkpoint || result.Bid.Checkpoint != checkpoint || result.Ask.Checkpoint != checkpoint || len(simulator.request.Transaction.Commands) != 2 {
		t.Fatalf("QuotePair() = %+v commands=%d", result, len(simulator.request.Transaction.Commands))
	}
}

func TestFlashSwapPTB(t *testing.T) {
	poolAddress, _ := onchainSui.ParseAddress("0x9")
	recipient, _ := onchainSui.ParseAddress("0xa")
	pool := Pool{Address: poolAddress, InitialVersion: 3, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC", FeeType: "0x1::fee::FEE"}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	flash, err := AppendFlashSwap(builder, testDeployment(), FlashSwapParams{Pool: pool, Recipient: recipient, A2B: true, Amount: big.NewInt(100), AmountSpecifiedIsInput: false, SqrtPriceLimit: big.NewInt(5)})
	if err != nil {
		t.Fatalf("AppendFlashSwap() returned an unexpected error: %v", err)
	}
	if err := AppendRepayFlashSwap(builder, testDeployment(), pool, flash); err != nil {
		t.Fatalf("AppendRepayFlashSwap() returned an unexpected error: %v", err)
	}
	tx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned an unexpected error: %v", err)
	}
	if len(tx.Commands) != 2 || tx.Commands[0].MoveCall.Function != "flash_swap" || tx.Commands[1].MoveCall.Function != "repay_flash_swap" {
		t.Fatalf("transaction=%+v", tx)
	}
	if len(tx.Inputs) != 8 || tx.Inputs[4].Pure[0] != 0 {
		t.Fatalf("transaction_inputs=%+v", tx.Inputs)
	}
}

func TestAtomicSwapPTB(t *testing.T) {
	poolAddress, _ := onchainSui.ParseAddress("0x9")
	recipient, _ := onchainSui.ParseAddress("0xa")
	packageAddress, _ := onchainSui.ParseAddress("0x2")
	pool := Pool{Address: poolAddress, InitialVersion: 3, CoinTypeA: "0x2::sui::SUI", CoinTypeB: "0x3::usdc::USDC", FeeType: "0x1::fee::FEE"}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	coin, err := builder.MoveCall(onchainSui.MoveCall{Package: packageAddress, Module: "coin", Function: "zero", TypeArguments: []string{pool.CoinTypeA}})
	if err != nil {
		t.Fatal(err)
	}
	amount, err := builder.MoveCall(onchainSui.MoveCall{Package: packageAddress, Module: "coin", Function: "value", TypeArguments: []string{pool.CoinTypeA}, Arguments: []onchainSui.Argument{coin}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := AppendAtomicSwap(builder, testDeployment(), AtomicSwapParams{Pool: pool, InputCoin: coin, AmountIn: amount, SqrtPriceLimit: big.NewInt(5), A2B: true, Recipient: recipient, DeadlineMS: 1000})
	if err != nil {
		t.Fatalf("AppendAtomicSwap() returned an unexpected error: %v", err)
	}
	tx, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.Commands) != 4 || tx.Commands[2].Kind != onchainSui.CommandKindMakeMoveVec || tx.Commands[3].MoveCall.Arguments[2] != amount || result.CoinA.Subresult == nil || *result.CoinA.Subresult != 1 || result.CoinB.Subresult == nil || *result.CoinB.Subresult != 0 {
		t.Fatalf("transaction=%+v result=%+v", tx, result)
	}
}

func TestParseSwapEventPreservesCheckpoint(t *testing.T) {
	transaction, _ := onchainSui.ParseTransactionDigest("11111111111111111111111111111111")
	event := onchainSui.Event{
		Checkpoint:     42,
		SequenceNumber: 7,
		Transaction:    transaction,
		Type:           "0x1::pool::SwapEvent",
		JSON:           json.RawMessage(`{"pool":"0x9","recipient":"0xa","amount_a":"100","amount_b":"99","sqrt_price":"10","protocol_fee":"0","fee_amount":"1","a_to_b":true}`),
	}
	swap, err := ParseSwapEvent(event)
	if err != nil {
		t.Fatalf("ParseSwapEvent() returned an unexpected error: %v", err)
	}
	if swap.Checkpoint != 42 || swap.SequenceNumber != 7 {
		t.Fatalf("ParseSwapEvent() = %+v", swap)
	}
}

func testDeployment() Deployment {
	packageAddress, _ := onchainSui.ParseAddress("0x1")
	version, _ := onchainSui.ParseAddress("0x2")
	clock, _ := onchainSui.ParseAddress("0x6")
	return Deployment{Package: packageAddress, PublishedAt: packageAddress, Versioned: onchainSui.ObjectInput{Address: version, Version: 1}, Clock: onchainSui.ObjectInput{Address: clock, Version: 1}, PoolModule: "pool", FetcherModule: "pool_fetcher", RouterModule: "swap_router"}
}
