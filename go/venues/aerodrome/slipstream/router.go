package slipstream

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const routerABIJSON = `[
{"inputs":[{"components":[{"name":"tokenIn","type":"address"},{"name":"tokenOut","type":"address"},{"name":"tickSpacing","type":"int24"},{"name":"recipient","type":"address"},{"name":"deadline","type":"uint256"},{"name":"amountIn","type":"uint256"},{"name":"amountOutMinimum","type":"uint256"},{"name":"sqrtPriceLimitX96","type":"uint160"}],"name":"params","type":"tuple"}],"name":"exactInputSingle","outputs":[{"name":"amountOut","type":"uint256"}],"stateMutability":"payable","type":"function"},
{"inputs":[{"components":[{"name":"tokenIn","type":"address"},{"name":"tokenOut","type":"address"},{"name":"tickSpacing","type":"int24"},{"name":"recipient","type":"address"},{"name":"deadline","type":"uint256"},{"name":"amountOut","type":"uint256"},{"name":"amountInMaximum","type":"uint256"},{"name":"sqrtPriceLimitX96","type":"uint160"}],"name":"params","type":"tuple"}],"name":"exactOutputSingle","outputs":[{"name":"amountIn","type":"uint256"}],"stateMutability":"payable","type":"function"}
]`

type ExactInputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	TickSpacing       *big.Int
	Recipient         common.Address
	Deadline          *big.Int
	AmountIn          *big.Int
	AmountOutMinimum  *big.Int
	SqrtPriceLimitX96 *big.Int
}

type ExactOutputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	TickSpacing       *big.Int
	Recipient         common.Address
	Deadline          *big.Int
	AmountOut         *big.Int
	AmountInMaximum   *big.Int
	SqrtPriceLimitX96 *big.Int
}

// EncodeExactInputSingle encodes an Aerodrome Slipstream exact-input Router call.
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
	return encodeRouterCall("exactInputSingle", params)
}

// EncodeExactOutputSingle encodes an Aerodrome Slipstream exact-output Router call.
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
	return encodeRouterCall("exactOutputSingle", params)
}

func encodeRouterCall(method string, params any) ([]byte, error) {
	parsed, err := abi.JSON(strings.NewReader(routerABIJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to encode aerodrome slipstream router call: failed to parse router abi: %w", err)
	}
	data, err := parsed.Pack(method, params)
	if err != nil {
		return nil, fmt.Errorf("failed to encode aerodrome slipstream router call: %w: method=%q", err, method)
	}
	return data, nil
}
