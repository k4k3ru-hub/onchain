package v3

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestEncodeSwapRouter02SinglePoolCalls(t *testing.T) {
	t.Parallel()
	input := ExactInputSingleParams{
		TokenIn: common.HexToAddress("0x1"), TokenOut: common.HexToAddress("0x2"), Fee: big.NewInt(3000),
		Recipient: common.HexToAddress("0x3"), AmountIn: big.NewInt(20), AmountOutMinimum: big.NewInt(19), SqrtPriceLimitX96: new(big.Int),
	}
	data, err := EncodeExactInputSingle(input)
	if err != nil || len(data) <= 4 {
		t.Fatalf("EncodeExactInputSingle() = (%x, %v)", data, err)
	}
	output := ExactOutputSingleParams{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, Fee: input.Fee,
		Recipient: input.Recipient, AmountOut: big.NewInt(20), AmountInMaximum: big.NewInt(21), SqrtPriceLimitX96: new(big.Int),
	}
	data, err = EncodeExactOutputSingle(output)
	if err != nil || len(data) <= 4 {
		t.Fatalf("EncodeExactOutputSingle() = (%x, %v)", data, err)
	}
}
