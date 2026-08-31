package clmm

import (
	"math/big"
	"testing"
)

func TestSqrtPriceAtTickMatchesOfficialBoundaryVectors(t *testing.T) {
	tests := []struct {
		tick int32
		want string
	}{
		{tick: minTick, want: "4295048016"},
		{tick: 0, want: "18446744073709551616"},
		{tick: maxTick, want: "79226673521066979257578248091"},
	}
	for _, test := range tests {
		got, err := sqrtPriceAtTick(test.tick)
		if err != nil {
			t.Fatalf("sqrtPriceAtTick(%d) error = %v", test.tick, err)
		}
		if got.String() != test.want {
			t.Fatalf("sqrtPriceAtTick(%d) = %s, want %s", test.tick, got, test.want)
		}
	}
}

func TestTickAtSqrtPriceRoundsDown(t *testing.T) {
	price, err := sqrtPriceAtTick(-28861)
	if err != nil {
		t.Fatalf("sqrtPriceAtTick() error = %v", err)
	}
	for _, test := range []struct {
		price *big.Int
		want  int32
	}{
		{price: price, want: -28861},
		{price: new(big.Int).Add(price, big.NewInt(1)), want: -28861},
		{price: new(big.Int).Sub(price, big.NewInt(1)), want: -28862},
	} {
		got, tickErr := tickAtSqrtPrice(test.price)
		if tickErr != nil {
			t.Fatalf("tickAtSqrtPrice() error = %v", tickErr)
		}
		if got != test.want {
			t.Fatalf("tickAtSqrtPrice() = %d, want %d", got, test.want)
		}
	}
}

func TestComputeExactInputStepChargesInputFee(t *testing.T) {
	current, _ := sqrtPriceAtTick(0)
	target, _ := sqrtPriceAtTick(-10)
	step, err := computeExactInputStep(current, target, big.NewInt(1_000_000_000), 1_000_000, 2500, true, true)
	if err != nil {
		t.Fatalf("computeExactInputStep() error = %v", err)
	}
	if step.amountIn+step.feeAmount > 1_000_000 || step.amountOut == 0 || step.sqrtPriceNext.Cmp(target) < 0 {
		t.Fatalf("computeExactInputStep() = %+v", step)
	}
}

func TestComputeExactInputLimitOrderMatchPartiallyConsumesOrder(t *testing.T) {
	price, err := sqrtPriceAtTick(0)
	if err != nil {
		t.Fatalf("sqrtPriceAtTick() error = %v", err)
	}
	match, err := computeExactInputLimitOrderMatch(price, 1_000, 2_000, 500, 2_500, true, true)
	if err != nil {
		t.Fatalf("computeExactInputLimitOrderMatch() error = %v", err)
	}
	if match.amountIn != 997 || match.amountOut != 997 || match.feeAmount != 3 || !match.ordersRemain {
		t.Fatalf("computeExactInputLimitOrderMatch() = %+v", match)
	}
}

func TestComputeExactInputLimitOrderMatchConsumesAllOrders(t *testing.T) {
	price, err := sqrtPriceAtTick(0)
	if err != nil {
		t.Fatalf("sqrtPriceAtTick() error = %v", err)
	}
	match, err := computeExactInputLimitOrderMatch(price, 2_000, 500, 0, 2_500, true, true)
	if err != nil {
		t.Fatalf("computeExactInputLimitOrderMatch() error = %v", err)
	}
	if match.amountIn != 500 || match.amountOut != 500 || match.feeAmount != 2 || match.ordersRemain {
		t.Fatalf("computeExactInputLimitOrderMatch() = %+v", match)
	}
}
