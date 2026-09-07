package slipstream

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
)

const (
	quoteExactInputSingleSignature  = "quoteExactInputSingle((address,address,uint256,int24,uint160))"
	quoteExactOutputSingleSignature = "quoteExactOutputSingle((address,address,uint256,int24,uint160))"
)

type QuoterConfig struct {
	Address common.Address
}

type QuoterClientParams struct {
	RPC    HTTPRPCClient
	Quoter QuoterConfig
}

type QuoterClient struct {
	rpc    HTTPRPCClient
	quoter common.Address
}

type QuoteExactInputSingleParams struct {
	PoolKey           protocol.PoolKey
	ZeroForOne        bool
	AmountIn          *big.Int
	SqrtPriceLimitX96 *big.Int
}

type QuoteExactInputSingleResult struct {
	AmountOut               *big.Int
	SqrtPriceX96After       *big.Int
	InitializedTicksCrossed uint32
	GasEstimate             *big.Int
}

type QuoteExactOutputSingleParams struct {
	PoolKey           protocol.PoolKey
	ZeroForOne        bool
	AmountOut         *big.Int
	SqrtPriceLimitX96 *big.Int
}

type QuoteExactOutputSingleResult struct {
	AmountIn                *big.Int
	SqrtPriceX96After       *big.Int
	InitializedTicksCrossed uint32
	GasEstimate             *big.Int
}

// NewQuoterClient creates a Slipstream QuoterV2 read client.
//
// Parameters:
//   - params: RPC dependency and QuoterV2 configuration.
//
// Returns:
//   - Quoter read client.
//   - Client creation error.
//
// Version:
//   - 2026-08-30: Added.
func NewQuoterClient(params QuoterClientParams) (*QuoterClient, error) {
	if params.RPC == nil {
		return nil, fmt.Errorf("failed to create slipstream quoter client: http_rpc_client=null")
	}
	if params.Quoter.Address == (common.Address{}) {
		return nil, fmt.Errorf("failed to create slipstream quoter client: quoter=empty")
	}
	return &QuoterClient{rpc: params.RPC, quoter: params.Quoter.Address}, nil
}

// QuoteExactInputSingle quotes a single-pool exact-input swap.
//
// Parameters:
//   - ctx: Request context.
//   - params: Pool and exact-input quote parameters in base units.
//   - blockNumber: State block; nil uses latest.
//
// Returns:
//   - Exact-input quote result.
//   - Quote error.
//
// Version:
//   - 2026-08-30: Added.
func (c *QuoterClient) QuoteExactInputSingle(ctx context.Context, params QuoteExactInputSingleParams, blockNumber *big.Int) (QuoteExactInputSingleResult, error) {
	response, err := c.quoteSingle(ctx, quoteExactInputSingleSignature, params.PoolKey, params.ZeroForOne, params.AmountIn, params.SqrtPriceLimitX96, "amount_in", blockNumber)
	if err != nil {
		return QuoteExactInputSingleResult{}, fmt.Errorf("failed to quote slipstream exact input single: %w", err)
	}
	result, err := decodeSingleQuoteResult(response)
	if err != nil {
		return QuoteExactInputSingleResult{}, fmt.Errorf("failed to quote slipstream exact input single: %w", err)
	}
	return QuoteExactInputSingleResult{
		AmountOut:               result.amount,
		SqrtPriceX96After:       result.sqrtPriceX96After,
		InitializedTicksCrossed: result.initializedTicksCrossed,
		GasEstimate:             result.gasEstimate,
	}, nil
}

// QuoteExactOutputSingle quotes a single-pool exact-output swap.
//
// Parameters:
//   - ctx: Request context.
//   - params: Pool and exact-output quote parameters in base units.
//   - blockNumber: State block; nil uses latest.
//
// Returns:
//   - Exact-output quote result.
//   - Quote error.
//
// Version:
//   - 2026-08-30: Added.
func (c *QuoterClient) QuoteExactOutputSingle(ctx context.Context, params QuoteExactOutputSingleParams, blockNumber *big.Int) (QuoteExactOutputSingleResult, error) {
	response, err := c.quoteSingle(ctx, quoteExactOutputSingleSignature, params.PoolKey, params.ZeroForOne, params.AmountOut, params.SqrtPriceLimitX96, "amount_out", blockNumber)
	if err != nil {
		return QuoteExactOutputSingleResult{}, fmt.Errorf("failed to quote slipstream exact output single: %w", err)
	}
	result, err := decodeSingleQuoteResult(response)
	if err != nil {
		return QuoteExactOutputSingleResult{}, fmt.Errorf("failed to quote slipstream exact output single: %w", err)
	}
	return QuoteExactOutputSingleResult{
		AmountIn:                result.amount,
		SqrtPriceX96After:       result.sqrtPriceX96After,
		InitializedTicksCrossed: result.initializedTicksCrossed,
		GasEstimate:             result.gasEstimate,
	}, nil
}

func (c *QuoterClient) quoteSingle(ctx context.Context, signature string, key protocol.PoolKey, zeroForOne bool, amount, sqrtPriceLimitX96 *big.Int, amountName string, blockNumber *big.Int) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to execute slipstream single quote: quoter_client=null")
	}
	if err := key.Validate(); err != nil {
		return nil, fmt.Errorf("failed to execute slipstream single quote: %w", err)
	}
	if err := validateUint(amount, 256, amountName, true); err != nil {
		return nil, fmt.Errorf("failed to execute slipstream single quote: %w", err)
	}
	if sqrtPriceLimitX96 == nil {
		sqrtPriceLimitX96 = new(big.Int)
	}
	if err := validateUint(sqrtPriceLimitX96, 160, "sqrt_price_limit_x96", false); err != nil {
		return nil, fmt.Errorf("failed to execute slipstream single quote: %w", err)
	}
	if err := validateBlockNumber(blockNumber); err != nil {
		return nil, fmt.Errorf("failed to execute slipstream single quote: %w", err)
	}

	tokenIn := key.Token0.Address()
	tokenOut := key.Token1.Address()
	if !zeroForOne {
		tokenIn, tokenOut = tokenOut, tokenIn
	}
	data := make([]byte, 4+32*5)
	copy(data[:4], methodSelector(signature))
	copy(data[4+12:4+32], tokenIn.Bytes())
	copy(data[4+32+12:4+64], tokenOut.Bytes())
	amount.FillBytes(data[4+64 : 4+96])
	new(big.Int).SetInt64(int64(key.TickSpacing)).FillBytes(data[4+96 : 4+128])
	sqrtPriceLimitX96.FillBytes(data[4+128 : 4+160])

	quoter := c.quoter
	response, err := c.rpc.CallContract(ctx, ethereum.CallMsg{To: &quoter, Data: data}, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to call slipstream quoter v2: %w", err)
	}
	return response, nil
}

type singleQuoteResult struct {
	amount                  *big.Int
	sqrtPriceX96After       *big.Int
	initializedTicksCrossed uint32
	gasEstimate             *big.Int
}

func decodeSingleQuoteResult(response []byte) (singleQuoteResult, error) {
	if len(response) != 32*4 {
		return singleQuoteResult{}, fmt.Errorf("failed to decode slipstream single quote result: response_length=invalid actual_length=%d expected_length=%d", len(response), 32*4)
	}
	amount := new(big.Int).SetBytes(response[0:32])
	sqrtPriceX96After := new(big.Int).SetBytes(response[32:64])
	initializedTicksCrossed := new(big.Int).SetBytes(response[64:96])
	gasEstimate := new(big.Int).SetBytes(response[96:128])
	if amount.Sign() <= 0 {
		return singleQuoteResult{}, fmt.Errorf("failed to decode slipstream single quote result: amount=out_of_range min_value=1")
	}
	if sqrtPriceX96After.Sign() <= 0 || sqrtPriceX96After.BitLen() > 160 {
		return singleQuoteResult{}, fmt.Errorf("failed to decode slipstream single quote result: sqrt_price_x96_after=out_of_range")
	}
	if initializedTicksCrossed.BitLen() > 32 {
		return singleQuoteResult{}, fmt.Errorf("failed to decode slipstream single quote result: initialized_ticks_crossed=out_of_range")
	}
	if gasEstimate.Sign() <= 0 {
		return singleQuoteResult{}, fmt.Errorf("failed to decode slipstream single quote result: gas_estimate=out_of_range min_value=1")
	}
	return singleQuoteResult{
		amount:                  amount,
		sqrtPriceX96After:       sqrtPriceX96After,
		initializedTicksCrossed: uint32(initializedTicksCrossed.Uint64()),
		gasEstimate:             gasEstimate,
	}, nil
}

func validateUint(value *big.Int, bits int, name string, positive bool) error {
	if value == nil {
		return fmt.Errorf("failed to validate unsigned integer: %s=null", name)
	}
	if value.Sign() < 0 || value.BitLen() > bits || positive && value.Sign() == 0 {
		return fmt.Errorf("failed to validate unsigned integer: %s=out_of_range", name)
	}
	return nil
}
