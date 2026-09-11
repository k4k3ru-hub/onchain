package clmm

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
	Package       onchainSui.Address
	PublishedAt   onchainSui.Address
	Versioned     onchainSui.ObjectInput
	Clock         onchainSui.ObjectInput
	PoolModule    string
	FetcherModule string
	RouterModule  string
}

// Validate validates a Turbos CLMM deployment.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-30: Added.
func (d Deployment) Validate() error {
	if d.Package.IsZero() || d.PublishedAt.IsZero() {
		return fmt.Errorf("failed to validate turbos clmm deployment: package=empty")
	}
	if d.Versioned.Address.IsZero() || d.Versioned.Version == 0 {
		return fmt.Errorf("failed to validate turbos clmm deployment: versioned=invalid")
	}
	if d.Clock.Address.IsZero() || d.Clock.Version == 0 {
		return fmt.Errorf("failed to validate turbos clmm deployment: clock=invalid")
	}
	if strings.TrimSpace(d.PoolModule) == "" || strings.TrimSpace(d.FetcherModule) == "" || strings.TrimSpace(d.RouterModule) == "" {
		return fmt.Errorf("failed to validate turbos clmm deployment: module=empty")
	}
	return nil
}

type SwapExactInputParams struct {
	Pool           Pool
	InputCoin      onchainSui.Argument
	AmountIn       uint64
	MinimumOut     uint64
	SqrtPriceLimit *big.Int
	A2B            bool
	Recipient      onchainSui.Address
	DeadlineMS     uint64
}

type SwapArguments struct {
	CoinA onchainSui.Argument
	CoinB onchainSui.Argument
}

// AppendSwapExactInput appends a direction-specific Turbos exact-input swap.
//
// The input coin is wrapped with the standard Sui MakeMoveVec command required
// by the Turbos swap router. Returned arguments can be consumed by later PTB
// commands in an atomic route.
//
// Parameters:
//   - builder: Programmable transaction builder.
//   - deployment: Turbos deployment.
//   - params: Swap parameters.
//
// Returns:
//   - Returned coin arguments.
//   - Validation error.
//
// Version:
//   - 2026-09-01: Corrected returned coin arguments for direction-specific router functions.
//   - 2026-08-31: Added.
func AppendSwapExactInput(builder *onchainSui.ProgrammableTransactionBuilder, deployment Deployment, params SwapExactInputParams) (SwapArguments, error) {
	if builder == nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: builder=null")
	}
	if err := deployment.Validate(); err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: %w", err)
	}
	if params.Pool.Address.IsZero() || params.Pool.InitialVersion == 0 || params.AmountIn == 0 || params.MinimumOut == 0 || !validU128(params.SqrtPriceLimit) || params.Recipient.IsZero() || params.DeadlineMS == 0 {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: parameters=invalid")
	}
	inputType := params.Pool.CoinTypeA
	function := "swap_a_b_with_return_"
	if !params.A2B {
		inputType = params.Pool.CoinTypeB
		function = "swap_b_a_with_return_"
	}
	coinVector, err := builder.MakeMoveVec(onchainSui.MakeMoveVec{ElementType: "0x2::coin::Coin<" + inputType + ">", Elements: []onchainSui.Argument{params.InputCoin}})
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: %w", err)
	}
	pool, err := builder.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: params.Pool.Address, Version: params.Pool.InitialVersion, Mutable: true})
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: %w", err)
	}
	amount, _ := builder.Pure(bcsUint64(params.AmountIn))
	minimum, _ := builder.Pure(bcsUint64(params.MinimumOut))
	limit, _ := builder.Pure(bcsUint128(params.SqrtPriceLimit))
	exact, _ := builder.Pure(bcsBool(true))
	recipient, _ := builder.Pure(bcsAddress(params.Recipient))
	deadline, _ := builder.Pure(bcsUint64(params.DeadlineMS))
	clock := deployment.Clock
	clock.Mutable = false
	clockArg, err := builder.Object(onchainSui.InputKindShared, clock)
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: %w", err)
	}
	version := deployment.Versioned
	version.Mutable = false
	versionArg, err := builder.Object(onchainSui.InputKindShared, version)
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: %w", err)
	}
	result, err := builder.MoveCall(onchainSui.MoveCall{Package: deployment.PublishedAt, Module: deployment.RouterModule, Function: function, TypeArguments: []string{params.Pool.CoinTypeA, params.Pool.CoinTypeB, params.Pool.FeeType}, Arguments: []onchainSui.Argument{pool, coinVector, amount, minimum, limit, exact, recipient, deadline, clockArg, versionArg}})
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: %w", err)
	}
	firstCoin, err := onchainSui.NestedResult(result, 0)
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: %w", err)
	}
	secondCoin, err := onchainSui.NestedResult(result, 1)
	if err != nil {
		return SwapArguments{}, fmt.Errorf("failed to append turbos clmm exact input swap: %w", err)
	}
	if params.A2B {
		return SwapArguments{CoinA: secondCoin, CoinB: firstCoin}, nil
	}
	return SwapArguments{CoinA: firstCoin, CoinB: secondCoin}, nil
}

type Pool struct {
	Address          onchainSui.Address
	InitialVersion   uint64
	CoinTypeA        string
	CoinTypeB        string
	FeeType          string
	CoinA            uint64
	CoinB            uint64
	SqrtPrice        *big.Int
	Liquidity        *big.Int
	TickCurrentIndex int32
	TickSpacing      uint32
	Fee              uint32
	Unlocked         bool
}

// ParsePool parses a Turbos CLMM pool Move object.
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
		return nil, fmt.Errorf("failed to parse turbos clmm pool: object=null")
	}
	types, err := moveTypeArguments(object.Move.Type)
	if err != nil || len(types) != 3 || !strings.Contains(object.Move.Type, "::pool::Pool<") {
		return nil, fmt.Errorf("failed to parse turbos clmm pool: move_type=invalid")
	}
	var value struct {
		CoinA     json.RawMessage `json:"coin_a"`
		CoinB     json.RawMessage `json:"coin_b"`
		SqrtPrice json.RawMessage `json:"sqrt_price"`
		Liquidity json.RawMessage `json:"liquidity"`
		Tick      struct {
			Bits uint32 `json:"bits"`
		} `json:"tick_current_index"`
		TickSpacing uint32 `json:"tick_spacing"`
		Fee         uint32 `json:"fee"`
		Unlocked    bool   `json:"unlocked"`
	}
	if err := json.Unmarshal(object.Move.JSON, &value); err != nil {
		return nil, fmt.Errorf("failed to parse turbos clmm pool: failed to decode object: %w", err)
	}
	coinA, err := jsonUint64(value.CoinA)
	if err != nil {
		return nil, fmt.Errorf("failed to parse turbos clmm pool: coin_a=invalid")
	}
	coinB, err := jsonUint64(value.CoinB)
	if err != nil {
		return nil, fmt.Errorf("failed to parse turbos clmm pool: coin_b=invalid")
	}
	sqrt, err := jsonUnsigned(value.SqrtPrice)
	if err != nil {
		return nil, fmt.Errorf("failed to parse turbos clmm pool: sqrt_price=invalid")
	}
	liquidity, err := jsonUnsigned(value.Liquidity)
	if err != nil {
		return nil, fmt.Errorf("failed to parse turbos clmm pool: liquidity=invalid")
	}
	return &Pool{Address: object.Address, InitialVersion: object.Version, CoinTypeA: types[0], CoinTypeB: types[1], FeeType: types[2], CoinA: coinA, CoinB: coinB, SqrtPrice: sqrt, Liquidity: liquidity, TickCurrentIndex: int32(value.Tick.Bits), TickSpacing: value.TickSpacing, Fee: value.Fee, Unlocked: value.Unlocked}, nil
}

type Simulator interface {
	SimulateTransaction(context.Context, onchainSui.SimulationRequest) (*onchainSui.SimulationResult, error)
}
type QuoteExactInputParams struct {
	Sender         onchainSui.Address
	Pool           Pool
	AmountIn       *big.Int
	A2B            bool
	SqrtPriceLimit *big.Int
}
type QuoteExactOutputParams struct {
	Sender         onchainSui.Address
	Pool           Pool
	AmountOut      *big.Int
	A2B            bool
	SqrtPriceLimit *big.Int
}
type QuoteResult struct {
	AmountIn       *big.Int
	AmountOut      *big.Int
	FeeAmount      uint64
	ProtocolFee    uint64
	AfterSqrtPrice *big.Int
	Checkpoint     onchainSui.CheckpointSequenceNumber
}
type Quoter struct {
	deployment Deployment
	simulator  Simulator
}

// NewQuoter creates a simulation-backed Turbos CLMM quoter.
//
// Parameters:
//   - deployment: Turbos deployment.
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
		return nil, fmt.Errorf("failed to create turbos clmm quoter: %w", err)
	}
	if simulator == nil {
		return nil, fmt.Errorf("failed to create turbos clmm quoter: simulator=null")
	}
	return &Quoter{deployment: deployment, simulator: simulator}, nil
}

// QuoteExactInput simulates Turbos pool_fetcher::compute_swap_result.
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
//   - 2026-08-30: Added.
func (q *Quoter) QuoteExactInput(ctx context.Context, params QuoteExactInputParams) (QuoteResult, error) {
	if q == nil || q.simulator == nil {
		return QuoteResult{}, fmt.Errorf("failed to quote turbos clmm exact input: quoter=null")
	}
	if params.Sender.IsZero() || params.Pool.Address.IsZero() || params.Pool.InitialVersion == 0 || !validU128(params.AmountIn) || !validU128(params.SqrtPriceLimit) {
		return QuoteResult{}, fmt.Errorf("failed to quote turbos clmm exact input: parameters=invalid")
	}
	return q.quote(ctx, params.Sender, params.Pool, params.AmountIn, params.A2B, true, params.SqrtPriceLimit, "failed to quote turbos clmm exact input")
}

// QuoteExactOutput simulates Turbos pool_fetcher::compute_swap_result for an exact output.
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
//   - 2026-09-01: Added.
func (q *Quoter) QuoteExactOutput(ctx context.Context, params QuoteExactOutputParams) (QuoteResult, error) {
	if q == nil || q.simulator == nil {
		return QuoteResult{}, fmt.Errorf("failed to quote turbos clmm exact output: quoter=null")
	}
	if params.Sender.IsZero() || params.Pool.Address.IsZero() || params.Pool.InitialVersion == 0 || !validU128(params.AmountOut) || !validU128(params.SqrtPriceLimit) {
		return QuoteResult{}, fmt.Errorf("failed to quote turbos clmm exact output: parameters=invalid")
	}
	return q.quote(ctx, params.Sender, params.Pool, params.AmountOut, params.A2B, false, params.SqrtPriceLimit, "failed to quote turbos clmm exact output")
}

func (q *Quoter) quote(ctx context.Context, sender onchainSui.Address, poolState Pool, amountValue *big.Int, aToB bool, exactInput bool, sqrtPriceLimit *big.Int, operation string) (QuoteResult, error) {
	b := onchainSui.NewProgrammableTransactionBuilder()
	pool, err := b.Object(onchainSui.InputKindShared, onchainSui.ObjectInput{Address: poolState.Address, Version: poolState.InitialVersion})
	if err != nil {
		return QuoteResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	a2b, _ := b.Pure(bcsBool(aToB))
	amount, _ := b.Pure(bcsUint128(amountValue))
	exact, _ := b.Pure(bcsBool(exactInput))
	limit, _ := b.Pure(bcsUint128(sqrtPriceLimit))
	clock := q.deployment.Clock
	clock.Mutable = false
	clockArg, err := b.Object(onchainSui.InputKindShared, clock)
	if err != nil {
		return QuoteResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	version := q.deployment.Versioned
	version.Mutable = false
	versionArg, err := b.Object(onchainSui.InputKindShared, version)
	if err != nil {
		return QuoteResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	_, err = b.MoveCall(onchainSui.MoveCall{Package: q.deployment.PublishedAt, Module: q.deployment.FetcherModule, Function: "compute_swap_result", TypeArguments: []string{poolState.CoinTypeA, poolState.CoinTypeB, poolState.FeeType}, Arguments: []onchainSui.Argument{pool, a2b, amount, exact, limit, clockArg, versionArg}})
	if err != nil {
		return QuoteResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	tx, err := b.Build()
	if err != nil {
		return QuoteResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	simulation, err := q.simulator.SimulateTransaction(ctx, onchainSui.SimulationRequest{Sender: sender, Transaction: tx})
	if err != nil {
		return QuoteResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	for _, event := range simulation.Events {
		if strings.HasSuffix(event.Type, "::pool::SwapEvent") {
			result, err := parseQuoteEvent(event.BCS, aToB)
			if err != nil {
				return QuoteResult{}, fmt.Errorf("%s: %w", operation, err)
			}
			result.Checkpoint = simulation.Checkpoint
			return result, nil
		}
	}
	return QuoteResult{}, fmt.Errorf("%s: swap_event=null", operation)
}

func parseQuoteEvent(value []byte, a2b bool) (QuoteResult, error) {
	if len(value) < 138 {
		return QuoteResult{}, fmt.Errorf("failed to parse turbos clmm quote event: bcs=too_short actual_length=%d min_length=138", len(value))
	}
	amountA, amountB := new(big.Int).SetUint64(binary.LittleEndian.Uint64(value[64:72])), new(big.Int).SetUint64(binary.LittleEndian.Uint64(value[72:80]))
	result := QuoteResult{FeeAmount: binary.LittleEndian.Uint64(value[128:136]), ProtocolFee: binary.LittleEndian.Uint64(value[120:128]), AfterSqrtPrice: littleEndianUint(value[104:120])}
	if a2b {
		result.AmountIn, result.AmountOut = amountA, amountB
	} else {
		result.AmountIn, result.AmountOut = amountB, amountA
	}
	if result.AmountIn.Sign() == 0 || result.AmountOut.Sign() == 0 {
		return QuoteResult{}, fmt.Errorf("failed to parse turbos clmm quote event: amount=invalid")
	}
	return result, nil
}

type Swap struct {
	// TransactionIndex is checkpoint-local; nil means unavailable, not index zero.
	TransactionIndex *uint64
	Checkpoint       onchainSui.CheckpointSequenceNumber
	SequenceNumber   uint64
	Transaction      onchainSui.TransactionDigest
	EventIndex       uint32
	Timestamp        time.Time
	Pool             onchainSui.Address
	Recipient        onchainSui.Address
	A2B              bool
	AmountA          uint64
	AmountB          uint64
	FeeAmount        uint64
	ProtocolFee      uint64
	SqrtPrice        string
}

// ParseSwapEvent parses a historical Turbos SwapEvent.
//
// Parameters:
//   - event: Sui event.
//
// Returns:
//   - Parsed swap.
//   - Parse error.
//
// Version:
//   - 2026-09-10: Preserve checkpoint-local transaction ordering.
//   - 2026-08-30: Added.
func ParseSwapEvent(event onchainSui.Event) (Swap, error) {
	swap, err := parseSwapJSON(event.Type, event.JSON)
	if err != nil {
		return Swap{}, err
	}
	swap.Checkpoint, swap.SequenceNumber, swap.Transaction, swap.Timestamp = event.Checkpoint, event.SequenceNumber, event.Transaction, event.Timestamp
	if event.TransactionIndex != nil {
		index := *event.TransactionIndex
		swap.TransactionIndex = &index
	}
	return swap, nil
}

// ParseLiveSwapEvent parses a live Turbos SwapEvent.
//
// Parameters:
//   - event: Sui live event.
//
// Returns:
//   - Parsed swap.
//   - Parse error.
//
// Version:
//   - 2026-09-11: Decode BCS-only transaction events locally.
//   - 2026-09-10: Preserve checkpoint-local transaction ordering.
//   - 2026-08-30: Added.
func ParseLiveSwapEvent(event onchainSui.LiveEvent) (Swap, error) {
	if len(event.JSON) == 0 {
		data, err := decodeSwapBCS(event.BCS)
		if err != nil {
			return Swap{}, fmt.Errorf("failed to parse live swap event: %w", err)
		}
		event.JSON = data
	}
	swap, err := parseSwapJSON(event.Type, event.JSON)
	if err != nil {
		return Swap{}, err
	}
	swap.Checkpoint, swap.Transaction, swap.EventIndex = event.Checkpoint, event.Transaction, event.EventIndex
	index := event.TransactionIndex
	swap.TransactionIndex = &index
	return swap, nil
}

func parseSwapJSON(eventType string, data json.RawMessage) (Swap, error) {
	if !strings.HasSuffix(eventType, "::pool::SwapEvent") {
		return Swap{}, fmt.Errorf("failed to parse turbos clmm swap event: event_type=invalid")
	}
	var v struct {
		Pool      string          `json:"pool"`
		Recipient string          `json:"recipient"`
		AmountA   json.RawMessage `json:"amount_a"`
		AmountB   json.RawMessage `json:"amount_b"`
		Sqrt      json.RawMessage `json:"sqrt_price"`
		Protocol  json.RawMessage `json:"protocol_fee"`
		Fee       json.RawMessage `json:"fee_amount"`
		A2B       bool            `json:"a_to_b"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return Swap{}, fmt.Errorf("failed to parse turbos clmm swap event: failed to decode event: %w", err)
	}
	pool, err := onchainSui.ParseAddress(v.Pool)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse turbos clmm swap event: pool=invalid: %w", err)
	}
	recipient, err := onchainSui.ParseAddress(v.Recipient)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse turbos clmm swap event: recipient=invalid: %w", err)
	}
	a, err := jsonUint64(v.AmountA)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse turbos clmm swap event: amount_a=invalid")
	}
	bb, err := jsonUint64(v.AmountB)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse turbos clmm swap event: amount_b=invalid")
	}
	fee, err := jsonUint64(v.Fee)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse turbos clmm swap event: fee=invalid")
	}
	protocol, err := jsonUint64(v.Protocol)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse turbos clmm swap event: protocol_fee=invalid")
	}
	sqrt, err := jsonUnsigned(v.Sqrt)
	if err != nil {
		return Swap{}, fmt.Errorf("failed to parse turbos clmm swap event: sqrt_price=invalid")
	}
	return Swap{Pool: pool, Recipient: recipient, A2B: v.A2B, AmountA: a, AmountB: bb, FeeAmount: fee, ProtocolFee: protocol, SqrtPrice: sqrt.String()}, nil
}

func moveTypeArguments(value string) ([]string, error) {
	start, end := strings.IndexByte(value, '<'), strings.LastIndexByte(value, '>')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("move_type=invalid")
	}
	body := value[start+1 : end]
	var out []string
	depth, offset := 0, 0
	for i, r := range body {
		if r == '<' {
			depth++
		}
		if r == '>' {
			depth--
		}
		if r == ',' && depth == 0 {
			out = append(out, strings.TrimSpace(body[offset:i]))
			offset = i + 1
		}
	}
	return append(out, strings.TrimSpace(body[offset:])), nil
}
func jsonUint64(value json.RawMessage) (uint64, error) {
	n, err := jsonUnsigned(value)
	if err != nil || !n.IsUint64() {
		return 0, fmt.Errorf("number=invalid")
	}
	return n.Uint64(), nil
}
func jsonUnsigned(value json.RawMessage) (*big.Int, error) {
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		var n json.Number
		if err := json.Unmarshal(value, &n); err != nil {
			return nil, err
		}
		s = n.String()
	}
	result, ok := new(big.Int).SetString(s, 10)
	if !ok || result.Sign() < 0 {
		return nil, fmt.Errorf("number=invalid")
	}
	return result, nil
}
func validU128(value *big.Int) bool { return value != nil && value.Sign() > 0 && value.BitLen() <= 128 }
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
func bcsAddress(value onchainSui.Address) []byte { return value.Bytes() }
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
