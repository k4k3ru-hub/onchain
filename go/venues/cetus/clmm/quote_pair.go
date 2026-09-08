package clmm

import (
	"context"
	"fmt"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type QuotePairParams struct {
	Bid QuoteExactInputParams
	Ask QuoteExactOutputParams
}

type QuotePairResult struct {
	Bid        QuoteResult
	Ask        QuoteResult
	Checkpoint onchainSui.CheckpointSequenceNumber
}

// QuotePair simulates bid and ask quotes in one programmable transaction.
//
// Parameters:
//   - ctx: Request context.
//   - params: Bid and ask quote parameters.
//
// Returns:
//   - Quote pair sharing one simulation checkpoint.
//   - Quote or simulation error.
//
// Version:
//   - 2026-09-01: Added.
func (q *Quoter) QuotePair(ctx context.Context, params QuotePairParams) (QuotePairResult, error) {
	if q == nil || q.simulator == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: quoter=null")
	}
	if params.Bid.Sender != params.Ask.Sender || params.Bid.Pool.Address != params.Ask.Pool.Address || params.Bid.Pool.InitialVersion != params.Ask.Pool.InitialVersion || params.Bid.AmountIn == 0 || params.Ask.AmountOut == 0 {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: parameters=invalid")
	}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	if err := q.appendPairQuote(builder, params.Bid.Pool, params.Bid.AmountIn, params.Bid.A2B, true); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: %w", err)
	}
	if err := q.appendPairQuote(builder, params.Ask.Pool, params.Ask.AmountOut, params.Ask.A2B, false); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: %w", err)
	}
	transaction, err := builder.Build()
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: %w", err)
	}
	simulation, err := q.simulator.SimulateTransaction(ctx, onchainSui.SimulationRequest{Sender: params.Bid.Sender, Transaction: transaction})
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: %w", err)
	}
	if simulation == nil || len(simulation.CommandResults) != 2 {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: command_result=invalid")
	}
	bid, err := cetusPairQuoteResult(simulation, 0)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: bid=invalid: %w", err)
	}
	ask, err := cetusPairQuoteResult(simulation, 1)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: ask=invalid: %w", err)
	}
	bid.Checkpoint, ask.Checkpoint = simulation.Checkpoint, simulation.Checkpoint
	return QuotePairResult{Bid: bid, Ask: ask, Checkpoint: simulation.Checkpoint}, nil
}

func (q *Quoter) appendPairQuote(builder *onchainSui.ProgrammableTransactionBuilder, poolConfig Pool, amount uint64, a2b, byAmountIn bool) error {
	pool, err := builder.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: poolConfig.Address, Version: poolConfig.InitialVersion})
	if err != nil {
		return fmt.Errorf("failed to append cetus clmm pair quote: %w", err)
	}
	a2bArgument, _ := builder.Pure(bcsBool(a2b))
	byAmountInArgument, _ := builder.Pure(bcsBool(byAmountIn))
	amountArgument, _ := builder.Pure(bcsUint64(amount))
	_, err = builder.MoveCall(onchainSui.MoveCall{Package: q.deployment.FetcherPackage, Module: q.deployment.FetcherModule, Function: "calculate_swap_result", TypeArguments: []string{poolConfig.CoinTypeA, poolConfig.CoinTypeB}, Arguments: []onchainSui.Argument{pool, a2bArgument, byAmountInArgument, amountArgument}})
	if err != nil {
		return fmt.Errorf("failed to append cetus clmm pair quote: %w", err)
	}
	return nil
}

func cetusPairQuoteResult(simulation *onchainSui.SimulationResult, index int) (QuoteResult, error) {
	command := simulation.CommandResults[index]
	var value []byte
	if len(command.ReturnValues) > 0 {
		value = command.ReturnValues[0].BCS
	} else if len(simulation.Events) > index {
		value = simulation.Events[index].BCS
	}
	if len(value) == 0 {
		return QuoteResult{}, fmt.Errorf("failed to parse cetus clmm pair quote: result=empty command_index=%d", index)
	}
	result, err := parseCalculatedSwapResult(value)
	if err != nil {
		return QuoteResult{}, err
	}
	if result.AmountIn == 0 || result.AmountOut == 0 || result.IsExceed {
		return QuoteResult{}, fmt.Errorf("failed to parse cetus clmm pair quote: quote=invalid command_index=%d", index)
	}
	return result, nil
}

// QuotePair simulates bid and ask Cetus quotes in one programmable transaction.
//
// Parameters:
//   - ctx: Request context.
//   - params: Quote-pair parameters.
//
// Returns:
//   - Quote pair sharing one checkpoint.
//   - Quote or simulation error.
//
// Version:
//   - 2026-09-01: Added.
func (c *Client) QuotePair(ctx context.Context, params QuotePairParams) (QuotePairResult, error) {
	if c == nil || c.quoter == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote cetus clmm pair: client=null")
	}
	return c.quoter.QuotePair(ctx, params)
}
