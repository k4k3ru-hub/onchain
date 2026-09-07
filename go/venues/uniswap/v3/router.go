package v3

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const swapRouter02ABIJSON = `[
{"inputs":[{"components":[{"name":"tokenIn","type":"address"},{"name":"tokenOut","type":"address"},{"name":"fee","type":"uint24"},{"name":"recipient","type":"address"},{"name":"amountIn","type":"uint256"},{"name":"amountOutMinimum","type":"uint256"},{"name":"sqrtPriceLimitX96","type":"uint160"}],"name":"params","type":"tuple"}],"name":"exactInputSingle","outputs":[{"name":"amountOut","type":"uint256"}],"stateMutability":"payable","type":"function"},
{"inputs":[{"components":[{"name":"tokenIn","type":"address"},{"name":"tokenOut","type":"address"},{"name":"fee","type":"uint24"},{"name":"recipient","type":"address"},{"name":"amountOut","type":"uint256"},{"name":"amountInMaximum","type":"uint256"},{"name":"sqrtPriceLimitX96","type":"uint160"}],"name":"params","type":"tuple"}],"name":"exactOutputSingle","outputs":[{"name":"amountIn","type":"uint256"}],"stateMutability":"payable","type":"function"}
]`

type ExactInputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	Fee               *big.Int
	Recipient         common.Address
	AmountIn          *big.Int
	AmountOutMinimum  *big.Int
	SqrtPriceLimitX96 *big.Int
}

type ExactOutputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	Fee               *big.Int
	Recipient         common.Address
	AmountOut         *big.Int
	AmountInMaximum   *big.Int
	SqrtPriceLimitX96 *big.Int
}

// EncodeExactInputSingle encodes a Uniswap V3 SwapRouter02 exact-input call.
//
// Parameters:
//   - params: Single-pool exact-input parameters.
//
// Returns:
//   - Router calldata.
//   - Encoding error.
//
// Version:
//   - 2026-09-08: Added.
func EncodeExactInputSingle(params ExactInputSingleParams) ([]byte, error) {
	return encodeSwapRouter02Call("exactInputSingle", params)
}

// EncodeExactOutputSingle encodes a Uniswap V3 SwapRouter02 exact-output call.
//
// Parameters:
//   - params: Single-pool exact-output parameters.
//
// Returns:
//   - Router calldata.
//   - Encoding error.
//
// Version:
//   - 2026-09-08: Added.
func EncodeExactOutputSingle(params ExactOutputSingleParams) ([]byte, error) {
	return encodeSwapRouter02Call("exactOutputSingle", params)
}

func encodeSwapRouter02Call(method string, params any) ([]byte, error) {
	parsed, err := abi.JSON(strings.NewReader(swapRouter02ABIJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to encode uniswap v3 swap router 02 call: failed to parse router abi: %w", err)
	}
	data, err := parsed.Pack(method, params)
	if err != nil {
		return nil, fmt.Errorf("failed to encode uniswap v3 swap router 02 call: %w: method=%q", err, method)
	}
	return data, nil
}
