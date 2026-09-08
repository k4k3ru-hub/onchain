package clmm

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type QuotePairParams struct {
	Bid QuoteExactInputParams
	Ask QuoteExactOutputParams
}

type QuotePairResult struct {
	StateTimestamp time.Time
	PoolVersion    uint64
	PoolDigest     onchainSui.ObjectDigest
	Bid            QuoteResult
	Ask            QuoteResult
	Checkpoint     onchainSui.CheckpointSequenceNumber
}

// QuotePair simulates bid and ask quotes in one programmable transaction.
//
// Parameters:
//   - ctx: Request context.
//   - params: Bid and ask quote parameters.
//
// Returns:
//   - Quote pair sharing one simulation checkpoint.
//   - Quote error.
//
// Version:
//   - 2026-09-08: Expose simulation input pool provenance.
//   - 2026-09-01: Added.
func (q *Quoter) QuotePair(ctx context.Context, params QuotePairParams) (QuotePairResult, error) {
	if q == nil || q.simulator == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: quoter=null")
	}
	if params.Bid.Sender != params.Ask.Sender || params.Bid.Pool.Address != params.Ask.Pool.Address || params.Bid.Pool.InitialVersion != params.Ask.Pool.InitialVersion {
		return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: parameters=invalid")
	}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	if err := q.appendPairQuote(builder, params.Bid.Pool, params.Bid.AmountIn, params.Bid.A2B, true, params.Bid.SqrtPriceLimit); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: %w", err)
	}
	if err := q.appendPairQuote(builder, params.Ask.Pool, params.Ask.AmountOut, params.Ask.A2B, false, params.Ask.SqrtPriceLimit); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: %w", err)
	}
	transaction, err := builder.Build()
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: %w", err)
	}
	simulation, err := q.simulator.SimulateTransaction(ctx, onchainSui.SimulationRequest{Sender: params.Bid.Sender, Transaction: transaction})
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: %w", err)
	}
	if simulation == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: simulation=null")
	}
	quotes := make([]QuoteResult, 0, 2)
	for _, event := range simulation.Events {
		if !strings.HasSuffix(event.Type, "::pool::SwapEvent") {
			continue
		}
		quote, parseErr := parseQuoteEvent(event.BCS, len(quotes) == 0 && params.Bid.A2B || len(quotes) == 1 && params.Ask.A2B)
		if parseErr != nil {
			return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: %w", parseErr)
		}
		quote.Checkpoint = simulation.Checkpoint
		quotes = append(quotes, quote)
	}
	if len(quotes) != 2 {
		return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: swap_event_count=invalid actual_count=%d expected_count=2", len(quotes))
	}
	var version uint64
	var digest onchainSui.ObjectDigest
	for _, input := range simulation.InputObjects {
		if input.Address == params.Bid.Pool.Address {
			version, digest = input.Version, input.Digest
			break
		}
	}
	return QuotePairResult{PoolVersion: version, PoolDigest: digest, Bid: quotes[0], Ask: quotes[1], Checkpoint: simulation.Checkpoint}, nil
}

func (q *Quoter) appendPairQuote(builder *onchainSui.ProgrammableTransactionBuilder, poolState Pool, amountValue *big.Int, aToB, exactInput bool, sqrtPriceLimit *big.Int) error {
	if builder == nil || !validU128(amountValue) || !validU128(sqrtPriceLimit) {
		return fmt.Errorf("failed to append turbos clmm pair quote: parameters=invalid")
	}
	pool, err := builder.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: poolState.Address, Version: poolState.InitialVersion})
	if err != nil {
		return fmt.Errorf("failed to append turbos clmm pair quote: %w", err)
	}
	a2b, _ := builder.Pure(bcsBool(aToB))
	amount, _ := builder.Pure(bcsUint128(amountValue))
	exact, _ := builder.Pure(bcsBool(exactInput))
	limit, _ := builder.Pure(bcsUint128(sqrtPriceLimit))
	clock := q.deployment.Clock
	clock.Mutable = false
	clockArg, err := builder.Object(onchainSui.InputKindShared, clock)
	if err != nil {
		return fmt.Errorf("failed to append turbos clmm pair quote: %w", err)
	}
	version := q.deployment.Versioned
	version.Mutable = false
	versionArg, err := builder.Object(onchainSui.InputKindShared, version)
	if err != nil {
		return fmt.Errorf("failed to append turbos clmm pair quote: %w", err)
	}
	_, err = builder.MoveCall(onchainSui.MoveCall{Package: q.deployment.PublishedAt, Module: q.deployment.FetcherModule, Function: "compute_swap_result", TypeArguments: []string{poolState.CoinTypeA, poolState.CoinTypeB, poolState.FeeType}, Arguments: []onchainSui.Argument{pool, a2b, amount, exact, limit, clockArg, versionArg}})
	if err != nil {
		return fmt.Errorf("failed to append turbos clmm pair quote: %w", err)
	}
	return nil
}

// QuotePair simulates bid and ask Turbos quotes in one programmable transaction.
//
// Parameters:
//   - ctx: Request context.
//   - params: Quote-pair parameters.
//
// Returns:
//   - Quote pair sharing one checkpoint.
//   - Quote error.
//
// Version:
//   - 2026-09-01: Added.
func (c *Client) QuotePair(ctx context.Context, params QuotePairParams) (QuotePairResult, error) {
	if c == nil || c.quoter == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote turbos clmm pair: client=null")
	}
	return c.quoter.QuotePair(ctx, params)
}
