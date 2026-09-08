package spot

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type Deployment struct {
	Package      onchainSui.Address
	PublishedAt  onchainSui.Address
	GlobalConfig onchainSui.ObjectInput
	Clock        onchainSui.ObjectInput
	PoolModule   string
	EventsModule string
}

// Validate validates a Bluefin Spot deployment.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func (d Deployment) Validate() error {
	if d.Package.IsZero() || d.PublishedAt.IsZero() {
		return fmt.Errorf("failed to validate bluefin spot deployment: package=empty")
	}
	if d.GlobalConfig.Address.IsZero() || d.GlobalConfig.Version == 0 {
		return fmt.Errorf("failed to validate bluefin spot deployment: global_config=invalid")
	}
	if d.Clock.Address.IsZero() || d.Clock.Version == 0 {
		return fmt.Errorf("failed to validate bluefin spot deployment: clock=invalid")
	}
	if strings.TrimSpace(d.PoolModule) == "" || strings.TrimSpace(d.EventsModule) == "" {
		return fmt.Errorf("failed to validate bluefin spot deployment: module=empty")
	}
	return nil
}

type Pool struct {
	Address        onchainSui.Address
	InitialVersion uint64
	CoinTypeA      string
	CoinTypeB      string
	CoinA          uint64
	CoinB          uint64
	SqrtPrice      *big.Int
	Liquidity      *big.Int
	FeeRate        uint64
	Paused         bool
}

// ParsePool parses a Bluefin Spot pool Move object.
//
// Parameters:
//   - object: Sui pool object.
//
// Returns:
//   - Parsed pool.
//   - Parse error.
//
// Version:
//   - 2026-08-30: Added.
func ParsePool(object *onchainSui.Object) (*Pool, error) {
	if object == nil || object.Move == nil {
		return nil, fmt.Errorf("failed to parse bluefin spot pool: object=null")
	}
	types, err := moveTypeArguments(object.Move.Type)
	if err != nil || len(types) != 2 || !strings.Contains(object.Move.Type, "::pool::Pool<") {
		return nil, fmt.Errorf("failed to parse bluefin spot pool: move_type=invalid")
	}
	var value struct {
		CoinA     json.RawMessage `json:"coin_a"`
		CoinB     json.RawMessage `json:"coin_b"`
		SqrtPrice json.RawMessage `json:"current_sqrt_price"`
		Liquidity json.RawMessage `json:"liquidity"`
		FeeRate   json.RawMessage `json:"fee_rate"`
		Paused    bool            `json:"is_paused"`
	}
	if err := json.Unmarshal(object.Move.JSON, &value); err != nil {
		return nil, fmt.Errorf("failed to parse bluefin spot pool: failed to decode object: %w", err)
	}
	coinA, err := jsonUint64(value.CoinA)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bluefin spot pool: coin_a=invalid")
	}
	coinB, err := jsonUint64(value.CoinB)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bluefin spot pool: coin_b=invalid")
	}
	sqrtPrice, err := jsonUnsigned(value.SqrtPrice)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bluefin spot pool: sqrt_price=invalid")
	}
	liquidity, err := jsonUnsigned(value.Liquidity)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bluefin spot pool: liquidity=invalid")
	}
	feeRate, err := jsonUint64(value.FeeRate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bluefin spot pool: fee_rate=invalid")
	}
	return &Pool{Address: object.Address, InitialVersion: object.Version, CoinTypeA: types[0], CoinTypeB: types[1], CoinA: coinA, CoinB: coinB, SqrtPrice: sqrtPrice, Liquidity: liquidity, FeeRate: feeRate, Paused: value.Paused}, nil
}

type Simulator interface {
	SimulateTransaction(context.Context, onchainSui.SimulationRequest) (*onchainSui.SimulationResult, error)
}

type QuoteExactInputParams struct {
	Sender         onchainSui.Address
	Pool           Pool
	AmountIn       uint64
	A2B            bool
	SqrtPriceLimit *big.Int
}

type QuoteExactOutputParams struct {
	Sender         onchainSui.Address
	Pool           Pool
	AmountOut      uint64
	A2B            bool
	SqrtPriceLimit *big.Int
}

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

type QuoteResult struct {
	AmountIn       uint64
	AmountOut      uint64
	FeeAmount      uint64
	ProtocolFee    uint64
	AfterSqrtPrice *big.Int
	IsExceed       bool
	Checkpoint     onchainSui.CheckpointSequenceNumber
}

type Quoter struct {
	deployment Deployment
	simulator  Simulator
}

// NewQuoter creates a simulation-backed Bluefin Spot quoter.
//
// Parameters:
//   - deployment: Bluefin deployment.
//   - simulator: Sui simulator.
//
// Returns:
//   - Quoter.
//   - Construction error.
//
// Version:
//   - 2026-08-30: Added.
func NewQuoter(deployment Deployment, simulator Simulator) (*Quoter, error) {
	if err := deployment.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create bluefin spot quoter: %w", err)
	}
	if simulator == nil {
		return nil, fmt.Errorf("failed to create bluefin spot quoter: simulator=null")
	}
	return &Quoter{deployment: deployment, simulator: simulator}, nil
}

// QuoteExactInput simulates Bluefin calculate_swap_results.
//
// Parameters:
//   - ctx: Request context.
//   - params: Quote parameters.
//
// Returns:
//   - Quote result.
//   - Quote error.
//
// Version:
//   - 2026-09-01: Returned the simulation checkpoint.
//   - 2026-08-31: Shared simulation with exact-output quotes.
//   - 2026-08-30: Added.
func (q *Quoter) QuoteExactInput(ctx context.Context, params QuoteExactInputParams) (QuoteResult, error) {
	result, err := q.quote(ctx, params.Sender, params.Pool, params.AmountIn, params.A2B, true, params.SqrtPriceLimit)
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot exact input: %w", err)
	}
	return result, nil
}

// QuoteExactOutput simulates Bluefin calculate_swap_results for one exact-output swap.
//
// Parameters:
//   - ctx: Request context.
//   - params: Quote parameters.
//
// Returns:
//   - Quote result.
//   - Quote error.
//
// Version:
//   - 2026-09-01: Returned the simulation checkpoint.
//   - 2026-08-31: Added.
func (q *Quoter) QuoteExactOutput(ctx context.Context, params QuoteExactOutputParams) (QuoteResult, error) {
	result, err := q.quote(ctx, params.Sender, params.Pool, params.AmountOut, params.A2B, false, params.SqrtPriceLimit)
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot exact output: %w", err)
	}
	return result, nil
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
//   - 2026-09-08: Expose exact input pool version and digest.
//   - 2026-09-01: Added.
func (q *Quoter) QuotePair(ctx context.Context, params QuotePairParams) (QuotePairResult, error) {
	if q == nil || q.simulator == nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin spot pair: quoter=null")
	}
	if params.Bid.Sender != params.Ask.Sender || params.Bid.Pool.Address != params.Ask.Pool.Address || params.Bid.Pool.InitialVersion != params.Ask.Pool.InitialVersion {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin spot pair: parameters=invalid")
	}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	if err := q.appendQuoteCall(builder, params.Bid.Sender, params.Bid.Pool, params.Bid.AmountIn, params.Bid.A2B, true, params.Bid.SqrtPriceLimit); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin spot pair: %w", err)
	}
	if err := q.appendQuoteCall(builder, params.Ask.Sender, params.Ask.Pool, params.Ask.AmountOut, params.Ask.A2B, false, params.Ask.SqrtPriceLimit); err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin spot pair: %w", err)
	}
	transaction, err := builder.Build()
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin spot pair: %w", err)
	}
	simulation, err := q.simulator.SimulateTransaction(ctx, onchainSui.SimulationRequest{Sender: params.Bid.Sender, Transaction: transaction})
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin spot pair: %w", err)
	}
	if simulation == nil || len(simulation.CommandResults) != 2 {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin spot pair: command_result=invalid")
	}
	bid, err := parseBluefinCommandQuote(simulation.CommandResults[0], true)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin spot pair: bid=invalid: %w", err)
	}
	ask, err := parseBluefinCommandQuote(simulation.CommandResults[1], false)
	if err != nil {
		return QuotePairResult{}, fmt.Errorf("failed to quote bluefin spot pair: ask=invalid: %w", err)
	}
	bid.Checkpoint, ask.Checkpoint = simulation.Checkpoint, simulation.Checkpoint
	var version uint64
	var digest onchainSui.ObjectDigest
	for _, input := range simulation.InputObjects {
		if input.Address == params.Bid.Pool.Address {
			version, digest = input.Version, input.Digest
			break
		}
	}
	return QuotePairResult{PoolVersion: version, PoolDigest: digest, Bid: bid, Ask: ask, Checkpoint: simulation.Checkpoint}, nil
}

func (q *Quoter) quote(ctx context.Context, sender onchainSui.Address, poolConfig Pool, amount uint64, a2b, byAmountIn bool, sqrtPriceLimit *big.Int) (QuoteResult, error) {
	if q == nil || q.simulator == nil {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot swap: quoter=null")
	}
	if sender.IsZero() || poolConfig.Address.IsZero() || poolConfig.InitialVersion == 0 || amount == 0 {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot swap: parameters=invalid")
	}
	if sqrtPriceLimit == nil || sqrtPriceLimit.Sign() <= 0 || sqrtPriceLimit.BitLen() > 128 {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot swap: sqrt_price_limit=invalid")
	}
	builder := onchainSui.NewProgrammableTransactionBuilder()
	if err := q.appendQuoteCall(builder, sender, poolConfig, amount, a2b, byAmountIn, sqrtPriceLimit); err != nil {
		return QuoteResult{}, err
	}
	transaction, err := builder.Build()
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot swap: %w", err)
	}
	simulation, err := q.simulator.SimulateTransaction(ctx, onchainSui.SimulationRequest{Sender: sender, Transaction: transaction})
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot swap: %w", err)
	}
	if simulation == nil || len(simulation.CommandResults) != 1 {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot swap: command_result=invalid")
	}
	result, err := parseBluefinCommandQuote(simulation.CommandResults[0], byAmountIn)
	if err != nil {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot swap: %w", err)
	}
	result.Checkpoint = simulation.Checkpoint
	return result, nil
}

func (q *Quoter) appendQuoteCall(builder *onchainSui.ProgrammableTransactionBuilder, sender onchainSui.Address, poolConfig Pool, amount uint64, a2b, byAmountIn bool, sqrtPriceLimit *big.Int) error {
	if builder == nil {
		return fmt.Errorf("failed to append bluefin spot quote: builder=null")
	}
	pool, err := builder.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: poolConfig.Address, Version: poolConfig.InitialVersion})
	if err != nil {
		return fmt.Errorf("failed to append bluefin spot quote: %w", err)
	}
	a2bArgument, _ := builder.Pure(bcsBool(a2b))
	byAmountInArgument, _ := builder.Pure(bcsBool(byAmountIn))
	amountArgument, _ := builder.Pure(bcsUint64(amount))
	limitArgument, _ := builder.Pure(bcsUint128(sqrtPriceLimit))
	_, err = builder.MoveCall(onchainSui.MoveCall{Package: q.deployment.PublishedAt, Module: q.deployment.PoolModule, Function: "calculate_swap_results", TypeArguments: []string{poolConfig.CoinTypeA, poolConfig.CoinTypeB}, Arguments: []onchainSui.Argument{pool, a2bArgument, byAmountInArgument, amountArgument, limitArgument}})
	if err != nil {
		return fmt.Errorf("failed to append bluefin spot quote: %w", err)
	}
	return nil
}

func parseBluefinCommandQuote(command onchainSui.SimulationCommandResult, byAmountIn bool) (QuoteResult, error) {
	if len(command.ReturnValues) == 0 {
		return QuoteResult{}, fmt.Errorf("failed to parse bluefin spot command quote: return_values=empty")
	}
	result, err := parseSwapResult(command.ReturnValues[0].BCS, byAmountIn)
	if err != nil {
		return QuoteResult{}, err
	}
	if result.AmountIn == 0 || result.AmountOut == 0 || result.IsExceed {
		return QuoteResult{}, fmt.Errorf("failed to quote bluefin spot swap: quote=invalid")
	}
	return result, nil
}

func parseSwapResult(value []byte, byAmountIn bool) (QuoteResult, error) {
	const fixedLength = 135
	if len(value) < fixedLength {
		return QuoteResult{}, fmt.Errorf("failed to parse bluefin spot swap result: bcs=too_short actual_length=%d min_length=%d", len(value), fixedLength)
	}
	specified := binary.LittleEndian.Uint64(value[2:10])
	remaining := binary.LittleEndian.Uint64(value[10:18])
	if remaining > specified {
		return QuoteResult{}, fmt.Errorf("failed to parse bluefin spot swap result: amount_remaining=invalid")
	}
	amountSpecified := specified - remaining
	amountCalculated := binary.LittleEndian.Uint64(value[18:26])
	amountIn, amountOut := amountCalculated, amountSpecified
	if byAmountIn {
		amountIn, amountOut = amountSpecified, amountCalculated
	}
	return QuoteResult{AmountIn: amountIn, AmountOut: amountOut, FeeAmount: binary.LittleEndian.Uint64(value[42:50]), ProtocolFee: binary.LittleEndian.Uint64(value[50:58]), AfterSqrtPrice: littleEndianUint(value[74:90]), IsExceed: value[94] != 0}, nil
}

type Swap struct {
	Checkpoint      onchainSui.CheckpointSequenceNumber
	SequenceNumber  uint64
	Transaction     onchainSui.TransactionDigest
	EventIndex      uint32
	Timestamp       time.Time
	Pool            onchainSui.Address
	A2B             bool
	AmountIn        uint64
	AmountOut       uint64
	FeeAmount       uint64
	BeforeSqrtPrice string
	AfterSqrtPrice  string
}

// ParseSwapEvent parses a historical Bluefin AssetSwap event.
//
// Parameters:
//   - event: Sui event.
//
// Returns:
//   - Parsed swap.
//   - Parse error.
//
// Version:
//   - 2026-08-31: Preserved the checkpoint from historical events.
//   - 2026-08-30: Added.
func ParseSwapEvent(event onchainSui.Event) (Swap, error) {
	swap, err := parseEvent(event.Type, event.JSON)
	if err != nil {
		return Swap{}, err
	}
	swap.Checkpoint, swap.SequenceNumber = event.Checkpoint, event.SequenceNumber
	swap.Transaction, swap.Timestamp = event.Transaction, event.Timestamp
	return swap, nil
}

// ParseLiveSwapEvent parses a live Bluefin AssetSwap event.
//
// Parameters:
//   - event: Sui live event.
//
// Returns:
//   - Parsed swap.
//   - Parse error.
//
// Version:
//   - 2026-08-30: Added.
func ParseLiveSwapEvent(event onchainSui.LiveEvent) (Swap, error) {
	swap, err := parseEvent(event.Type, event.JSON)
	if err != nil {
		return Swap{}, err
	}
	swap.Checkpoint, swap.Transaction, swap.EventIndex = event.Checkpoint, event.Transaction, event.EventIndex
	return swap, nil
}

func parseEvent(eventType string, data json.RawMessage) (Swap, error) {
	if !strings.HasSuffix(eventType, "::events::AssetSwap") {
		return Swap{}, fmt.Errorf("failed to parse bluefin spot swap event: event_type=invalid")
	}
	var value struct {
		Pool      string          `json:"pool_id"`
		A2B       bool            `json:"a2b"`
		AmountIn  json.RawMessage `json:"amount_in"`
		AmountOut json.RawMessage `json:"amount_out"`
		Fee       json.RawMessage `json:"fee"`
		Before    json.RawMessage `json:"before_sqrt_price"`
		After     json.RawMessage `json:"after_sqrt_price"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return Swap{}, fmt.Errorf("failed to parse bluefin spot swap event: failed to decode event: %w", err)
	}
	pool, err := onchainSui.ParseAddress(value.Pool)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse bluefin spot swap event: pool=invalid: %w", err)
	}
	amountIn, err := jsonUint64(value.AmountIn)
	if err != nil || amountIn == 0 {
		return Swap{}, fmt.Errorf("failed to parse bluefin spot swap event: amount_in=invalid")
	}
	amountOut, err := jsonUint64(value.AmountOut)
	if err != nil || amountOut == 0 {
		return Swap{}, fmt.Errorf("failed to parse bluefin spot swap event: amount_out=invalid")
	}
	fee, err := jsonUint64(value.Fee)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse bluefin spot swap event: fee=invalid")
	}
	before, err := jsonUnsigned(value.Before)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse bluefin spot swap event: before_sqrt_price=invalid")
	}
	after, err := jsonUnsigned(value.After)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse bluefin spot swap event: after_sqrt_price=invalid")
	}
	return Swap{Pool: pool, A2B: value.A2B, AmountIn: amountIn, AmountOut: amountOut, FeeAmount: fee, BeforeSqrtPrice: before.String(), AfterSqrtPrice: after.String()}, nil
}

func moveTypeArguments(value string) ([]string, error) {
	start, end := strings.IndexByte(value, '<'), strings.LastIndexByte(value, '>')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("move_type=invalid")
	}
	var result []string
	depth, offset := 0, 0
	body := value[start+1 : end]
	for i, r := range body {
		if r == '<' {
			depth++
		}
		if r == '>' {
			depth--
		}
		if r == ',' && depth == 0 {
			result = append(result, strings.TrimSpace(body[offset:i]))
			offset = i + 1
		}
	}
	result = append(result, strings.TrimSpace(body[offset:]))
	return result, nil
}

func jsonUint64(value json.RawMessage) (uint64, error) {
	number, err := jsonUnsigned(value)
	if err != nil || !number.IsUint64() {
		return 0, fmt.Errorf("number=invalid")
	}
	return number.Uint64(), nil
}
func jsonUnsigned(value json.RawMessage) (*big.Int, error) {
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		var number json.Number
		if err := json.Unmarshal(value, &number); err != nil {
			return nil, err
		}
		text = number.String()
	}
	result, ok := new(big.Int).SetString(text, 10)
	if !ok || result.Sign() < 0 {
		return nil, fmt.Errorf("number=invalid")
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
func littleEndianUint(value []byte) *big.Int {
	reversed := append([]byte(nil), value...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return new(big.Int).SetBytes(reversed)
}
