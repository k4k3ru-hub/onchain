package v3

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
)

const (
	quoteExactInputSingleSignature  = "quoteExactInputSingle((address,address,uint256,uint24,uint160))"
	quoteExactOutputSingleSignature = "quoteExactOutputSingle((address,address,uint256,uint24,uint160))"
)

type QuoteExactInputSingleParams struct {
	PoolKey           protocol.PoolKey
	ZeroForOne        bool
	AmountIn          *big.Int
	SqrtPriceLimitX96 *big.Int
}

type QuoteExactOutputSingleParams struct {
	PoolKey           protocol.PoolKey
	ZeroForOne        bool
	AmountOut         *big.Int
	SqrtPriceLimitX96 *big.Int
}

type QuoteExactInputSingleResult struct {
	AmountOut               *big.Int
	SqrtPriceX96After       *big.Int
	InitializedTicksCrossed uint32
	GasEstimate             *big.Int
}

type QuoteExactOutputSingleResult struct {
	AmountIn                *big.Int
	SqrtPriceX96After       *big.Int
	InitializedTicksCrossed uint32
	GasEstimate             *big.Int
}

type quoteSingleParamsABI struct {
	TokenIn           common.Address
	TokenOut          common.Address
	Amount            *big.Int
	Fee               *big.Int
	SqrtPriceLimitX96 *big.Int
}

// QuoteExactInputSingle quotes a single-pool exact-input swap through QuoterV2.
//
// Parameters:
//   - ctx: request context.
//   - params: pool and exact-input quote parameters in base units.
//   - blockNumber: block number; nil uses the latest block.
//
// Returns:
//   - Exact-input quote result.
//   - Quote error.
//
// Version:
//   - 2026-08-24: Added.
func (c *HTTPClient) QuoteExactInputSingle(ctx context.Context, params QuoteExactInputSingleParams, blockNumber *big.Int) (QuoteExactInputSingleResult, error) {
	const operation = "failed to quote uniswap v3 exact input single"
	response, err := c.callSingleQuote(ctx, quoteExactInputSingleSignature, params.PoolKey, params.ZeroForOne, params.AmountIn, params.SqrtPriceLimitX96, "amount_in", blockNumber)
	if err != nil {
		return QuoteExactInputSingleResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	amount, sqrtPrice, ticks, gas, err := decodeSingleQuoteResult(response, "amount_out")
	if err != nil {
		return QuoteExactInputSingleResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	return QuoteExactInputSingleResult{AmountOut: amount, SqrtPriceX96After: sqrtPrice, InitializedTicksCrossed: ticks, GasEstimate: gas}, nil
}

// QuoteExactOutputSingle quotes a single-pool exact-output swap through QuoterV2.
//
// Parameters:
//   - ctx: request context.
//   - params: pool and exact-output quote parameters in base units.
//   - blockNumber: block number; nil uses the latest block.
//
// Returns:
//   - Exact-output quote result.
//   - Quote error.
//
// Version:
//   - 2026-08-24: Added.
func (c *HTTPClient) QuoteExactOutputSingle(ctx context.Context, params QuoteExactOutputSingleParams, blockNumber *big.Int) (QuoteExactOutputSingleResult, error) {
	const operation = "failed to quote uniswap v3 exact output single"
	response, err := c.callSingleQuote(ctx, quoteExactOutputSingleSignature, params.PoolKey, params.ZeroForOne, params.AmountOut, params.SqrtPriceLimitX96, "amount_out", blockNumber)
	if err != nil {
		return QuoteExactOutputSingleResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	amount, sqrtPrice, ticks, gas, err := decodeSingleQuoteResult(response, "amount_in")
	if err != nil {
		return QuoteExactOutputSingleResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	return QuoteExactOutputSingleResult{AmountIn: amount, SqrtPriceX96After: sqrtPrice, InitializedTicksCrossed: ticks, GasEstimate: gas}, nil
}

func (c *HTTPClient) callSingleQuote(ctx context.Context, signature string, poolKey protocol.PoolKey, zeroForOne bool, amount, priceLimit *big.Int, amountName string, blockNumber *big.Int) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to validate uniswap v3 single quote: client=null")
	}
	if c.rpc == nil {
		return nil, fmt.Errorf("failed to validate uniswap v3 single quote: http_rpc_client=null")
	}
	if c.quoterV2 == (common.Address{}) {
		return nil, fmt.Errorf("failed to validate uniswap v3 single quote: quoter_v2=empty")
	}
	if err := poolKey.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate uniswap v3 single quote: %w", err)
	}
	configured := false
	for _, key := range c.poolKeys {
		if key.Token0 == poolKey.Token0 && key.Token1 == poolKey.Token1 && key.Fee == poolKey.Fee {
			configured = true
			break
		}
	}
	if !configured {
		return nil, fmt.Errorf("failed to validate uniswap v3 single quote: pool_key=unconfigured")
	}
	if amount == nil {
		return nil, fmt.Errorf("failed to validate uniswap v3 single quote: %s=null", amountName)
	}
	if amount.Sign() <= 0 || amount.BitLen() > 256 {
		return nil, fmt.Errorf("failed to validate uniswap v3 single quote: %s=out_of_range", amountName)
	}
	if priceLimit == nil {
		priceLimit = new(big.Int)
	}
	if priceLimit.Sign() < 0 || priceLimit.BitLen() > 160 {
		return nil, fmt.Errorf("failed to validate uniswap v3 single quote: sqrt_price_limit_x96=out_of_range")
	}
	if blockNumber != nil && blockNumber.Sign() < 0 {
		return nil, fmt.Errorf("failed to validate uniswap v3 single quote: block_number=out_of_range")
	}
	tokenIn, tokenOut := poolKey.Token0.Address(), poolKey.Token1.Address()
	if !zeroForOne {
		tokenIn, tokenOut = tokenOut, tokenIn
	}
	data, err := encodeSingleQuoteCall(signature, quoteSingleParamsABI{TokenIn: tokenIn, TokenOut: tokenOut, Amount: new(big.Int).Set(amount), Fee: new(big.Int).SetUint64(uint64(poolKey.Fee)), SqrtPriceLimitX96: new(big.Int).Set(priceLimit)})
	if err != nil {
		return nil, err
	}
	quoter := c.quoterV2
	response, err := c.rpc.CallContract(ctx, ethereum.CallMsg{To: &quoter, Data: data}, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to call uniswap v3 quoter v2: %w", err)
	}
	return response, nil
}

func encodeSingleQuoteCall(signature string, params quoteSingleParamsABI) ([]byte, error) {
	tuple, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{{Name: "tokenIn", Type: "address"}, {Name: "tokenOut", Type: "address"}, {Name: "amount", Type: "uint256"}, {Name: "fee", Type: "uint24"}, {Name: "sqrtPriceLimitX96", Type: "uint160"}})
	if err != nil {
		return nil, fmt.Errorf("failed to encode uniswap v3 single quote call: %w", err)
	}
	encoded, err := (abi.Arguments{{Type: tuple}}).Pack(params)
	if err != nil {
		return nil, fmt.Errorf("failed to encode uniswap v3 single quote call: %w", err)
	}
	return append(append([]byte(nil), crypto.Keccak256([]byte(signature))[:4]...), encoded...), nil
}

func decodeSingleQuoteResult(response []byte, amountName string) (*big.Int, *big.Int, uint32, *big.Int, error) {
	if len(response) == 0 {
		return nil, nil, 0, nil, fmt.Errorf("failed to decode uniswap v3 single quote result: response=empty")
	}
	types := make([]abi.Type, 4)
	for i, name := range []string{"uint256", "uint160", "uint32", "uint256"} {
		t, err := abi.NewType(name, "", nil)
		if err != nil {
			return nil, nil, 0, nil, fmt.Errorf("failed to decode uniswap v3 single quote result: %w", err)
		}
		types[i] = t
	}
	values, err := (abi.Arguments{{Type: types[0]}, {Type: types[1]}, {Type: types[2]}, {Type: types[3]}}).Unpack(response)
	if err != nil {
		return nil, nil, 0, nil, fmt.Errorf("failed to decode uniswap v3 single quote result: %w", err)
	}
	amount, ok := values[0].(*big.Int)
	if !ok || amount.Sign() <= 0 {
		return nil, nil, 0, nil, fmt.Errorf("failed to decode uniswap v3 single quote result: %s=invalid", amountName)
	}
	sqrtPrice, ok := values[1].(*big.Int)
	if !ok || sqrtPrice.Sign() < 0 || sqrtPrice.BitLen() > 160 {
		return nil, nil, 0, nil, fmt.Errorf("failed to decode uniswap v3 single quote result: sqrt_price_x96_after=invalid")
	}
	ticks, ok := values[2].(uint32)
	if !ok {
		return nil, nil, 0, nil, fmt.Errorf("failed to decode uniswap v3 single quote result: initialized_ticks_crossed=invalid")
	}
	gas, ok := values[3].(*big.Int)
	if !ok || gas.Sign() < 0 {
		return nil, nil, 0, nil, fmt.Errorf("failed to decode uniswap v3 single quote result: gas_estimate=invalid")
	}
	return new(big.Int).Set(amount), new(big.Int).Set(sqrtPrice), ticks, new(big.Int).Set(gas), nil
}
