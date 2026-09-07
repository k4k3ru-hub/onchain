package slipstream

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestEncodeRouterSinglePoolCalls(t *testing.T) {
	t.Parallel()
	input := ExactInputSingleParams{
		TokenIn: common.HexToAddress("0x1"), TokenOut: common.HexToAddress("0x2"), TickSpacing: big.NewInt(100),
		Recipient: common.HexToAddress("0x3"), Deadline: big.NewInt(10), AmountIn: big.NewInt(20), AmountOutMinimum: big.NewInt(19), SqrtPriceLimitX96: new(big.Int),
	}
	data, err := EncodeExactInputSingle(input)
	if err != nil || len(data) <= 4 {
		t.Fatalf("EncodeExactInputSingle() = (%x, %v)", data, err)
	}
	output := ExactOutputSingleParams{
		TokenIn: input.TokenIn, TokenOut: input.TokenOut, TickSpacing: input.TickSpacing,
		Recipient: input.Recipient, Deadline: input.Deadline, AmountOut: big.NewInt(20), AmountInMaximum: big.NewInt(21), SqrtPriceLimitX96: new(big.Int),
	}
	data, err = EncodeExactOutputSingle(output)
	if err != nil || len(data) <= 4 {
		t.Fatalf("EncodeExactOutputSingle() = (%x, %v)", data, err)
	}
}
