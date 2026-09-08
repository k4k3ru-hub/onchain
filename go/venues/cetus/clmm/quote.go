package clmm

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type Simulator interface {
	SimulateTransaction(context.Context, onchainSui.SimulationRequest) (*onchainSui.SimulationResult, error)
}

type QuoteExactInputParams struct {
	Sender   onchainSui.Address
	Pool     Pool
	AmountIn uint64
	A2B      bool
}

type QuoteExactOutputParams struct {
	Sender    onchainSui.Address
	Pool      Pool
	AmountOut uint64
	A2B       bool
}

type QuoteResult struct {
	AmountIn       uint64
	AmountOut      uint64
	FeeAmount      uint64
	FeeRate        uint64
	AfterSqrtPrice *big.Int
	IsExceed       bool
	Checkpoint     onchainSui.CheckpointSequenceNumber
}

type Quoter struct {
	deployment Deployment
	simulator  Simulator
}

// NewQuoter creates a Cetus CLMM simulation-backed quoter.
//
// Parameters:
//   - deployment: Cetus deployment.
//   - simulator: Sui transaction simulator.
//
// Returns:
//   - Quoter.
//   - Construction error.
//
// Version:
//   - 2026-08-31: Called the quote fetcher through its dedicated integrate package.
//   - 2026-08-30: Added.
func NewQuoter(deployment Deployment, simulator Simulator) (*Quoter, error) {
	if err := deployment.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create cetus clmm quoter: %w", err)
	}
	if simulator == nil {
		return nil, fmt.Errorf("failed to create cetus clmm quoter: simulator=null")
	}
	return &Quoter{deployment: deployment, simulator: simulator}, nil
}

// QuoteExactInput simulates Cetus calculate_swap_result for one exact-input swap.
//
// Parameters:
//   - ctx: Request context.
//   - params: Quote parameters.
//
// Returns:
//   - Quote result.
//   - Quote or simulation error.
//
// Version:
//   - 2026-09-01: Returned the simulation checkpoint.
//   - 2026-08-30: Added.
func (q *Quoter) QuoteExactInput(ctx context.Context, params QuoteExactInputParams) (QuoteResult, error) {
	return q.quote(ctx, params.Sender, params.Pool, params.AmountIn, params.A2B, true)
}

// QuoteExactOutput simulates Cetus calculate_swap_result for one exact-output swap.
//
// Parameters:
//   - ctx: Request context.
//   - params: Quote parameters.
//
// Returns:
//   - Quote result.
//   - Quote or simulation error.
//
// Version:
//   - 2026-09-01: Returned the simulation checkpoint.
//   - 2026-08-31: Added.
func (q *Quoter) QuoteExactOutput(ctx context.Context, params QuoteExactOutputParams) (QuoteResult, error) {
	return q.quote(ctx, params.Sender, params.Pool, params.AmountOut, params.A2B, false)
}

func (q *Quoter) quote(ctx context.Context, sender onchainSui.Address, poolConfig Pool, amount uint64, a2b, byAmountIn bool) (QuoteResult, error) {
	if q == nil || q.simulator == nil {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: quoter=null")
	}
	if sender.IsZero() {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: sender=empty")
	}
	if poolConfig.Address.IsZero() || poolConfig.InitialVersion == 0 {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: pool=invalid")
	}
	if amount == 0 {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: amount=empty")
	}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	pool, err := builder.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: poolConfig.Address, Version: poolConfig.InitialVersion})
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: %w", err)
	}
	a2bArgument, _ := builder.Pure(bcsBool(a2b))
	byAmountInArgument, _ := builder.Pure(bcsBool(byAmountIn))
	amountArgument, _ := builder.Pure(bcsUint64(amount))
	_, err = builder.MoveCall(onchainSui.MoveCall{
		Package: q.deployment.FetcherPackage, Module: q.deployment.FetcherModule, Function: "calculate_swap_result",
		TypeArguments: []string{poolConfig.CoinTypeA, poolConfig.CoinTypeB}, Arguments: []onchainSui.Argument{pool, a2bArgument, byAmountInArgument, amountArgument},
	})
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: %w", err)
	}
	transaction, err := builder.Build()
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: %w", err)
	}
	simulation, err := q.simulator.SimulateTransaction(ctx, onchainSui.SimulationRequest{Sender: sender, Transaction: transaction})
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: %w", err)
	}
	if simulation == nil {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: simulation=null")
	}
	if len(simulation.CommandResults) != 1 {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: command_result=invalid command_count=%d expected_command_count=1", len(simulation.CommandResults))
	}
	commandResult := simulation.CommandResults[0]
	if len(commandResult.ReturnValues) == 0 {
		if len(simulation.Events) == 1 && len(simulation.Events[0].BCS) > 0 {
			result, err := parseCalculatedSwapResult(simulation.Events[0].BCS)
			if err != nil {
				return QuoteResult{}, fmt.Errorf(
					"failed to quote cetus clmm swap: failed to parse simulation event result: %w: event_type=%q event_bcs_length=%d event_json_present=%t",
					err,
					simulation.Events[0].Type,
					len(simulation.Events[0].BCS),
					simulation.Events[0].JSON != nil,
				)
			}
			if result.AmountIn == 0 || result.AmountOut == 0 || result.IsExceed {
				return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: quote=invalid result_source=simulation_event")
			}
			result.Checkpoint = simulation.Checkpoint
			return result, nil
		}
		eventType, eventBCSLength, eventJSONPresent := "", 0, false
		if len(simulation.Events) > 0 {
			eventType = simulation.Events[0].Type
			eventBCSLength = len(simulation.Events[0].BCS)
			eventJSONPresent = simulation.Events[0].JSON != nil
		}
		return QuoteResult{}, fmt.Errorf(
			"failed to quote cetus clmm swap: command_result=invalid command_count=1 return_value_count=0 mutated_by_ref_count=%d event_count=%d first_event_type=%q first_event_bcs_length=%d first_event_json_present=%t",
			len(commandResult.MutatedByRef),
			len(simulation.Events),
			eventType,
			eventBCSLength,
			eventJSONPresent,
		)
	}
	returnValue := commandResult.ReturnValues[0]
	if len(returnValue.BCS) == 0 {
		return QuoteResult{}, fmt.Errorf(
			"failed to quote cetus clmm swap: command_result=invalid command_count=1 return_value_count=%d return_value_bcs=empty return_value_json_present=%t mutated_by_ref_count=%d event_count=%d",
			len(commandResult.ReturnValues),
			returnValue.JSON != nil,
			len(commandResult.MutatedByRef),
			len(simulation.Events),
		)
	}
	result, err := parseCalculatedSwapResult(returnValue.BCS)
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: %w", err)
	}
	if result.AmountIn == 0 || result.AmountOut == 0 || result.IsExceed {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm swap: quote=invalid")
	}
	result.Checkpoint = simulation.Checkpoint
	return result, nil
}

func parseCalculatedSwapResult(value []byte) (QuoteResult, error) {
	const fixedLength = 8*4 + 16 + 1
	if len(value) < fixedLength {
		return QuoteResult{}, fmt.Errorf("failed to parse cetus calculated swap result: bcs=too_short actual_length=%d min_length=%d", len(value), fixedLength)
	}
	result := QuoteResult{
		AmountIn: binary.LittleEndian.Uint64(value[0:8]), AmountOut: binary.LittleEndian.Uint64(value[8:16]),
		FeeAmount: binary.LittleEndian.Uint64(value[16:24]), FeeRate: binary.LittleEndian.Uint64(value[24:32]),
		AfterSqrtPrice: littleEndianUint128(value[32:48]), IsExceed: value[48] != 0,
	}
	return result, nil
}

func bcsBool(value bool) []byte {
	if value {
		return []byte{1}
	}
	return []byte{0}
}

func bcsUint64(value uint64) []byte {
	result := make([]byte, 8)
	binary.LittleEndian.PutUint64(result, value)
	return result
}

func bcsUint128(value *big.Int) []byte {
	result := make([]byte, 16)
	bytes := value.Bytes()
	for i := range bytes {
		result[i] = bytes[len(bytes)-1-i]
	}
	return result
}

func littleEndianUint128(value []byte) *big.Int {
	reversed := append([]byte(nil), value...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return new(big.Int).SetBytes(reversed)
}
