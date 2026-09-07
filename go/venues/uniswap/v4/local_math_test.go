package v4

import (
	"math/big"
	"testing"
)

func TestTickPriceBounds(t *testing.T) {
	for tick, want := range map[int32]string{-887272: "4295128739", 0: "79228162514264337593543950336", 887272: "1461446703485210103287273052203988822378723970342"} {
		if sqrtAtTick(tick).String() != want {
			t.Fatal(tick)
		}
	}
}

// Numerical reference vectors: Uniswap/v3-core test/SwapMath.spec.ts.
func TestSwapStepReferenceVectors(t *testing.T) {
	q := power2(96)
	l := integer("2000000000000000000")
	amount := integer("1000000000000000000")
	for _, tc := range []struct {
		input                bool
		target, in, out, fee string
	}{
		{true, "10", "999400000000000000", "666399946655997866", "600000000000000"},
		{false, "100", "2000000000000000000", "1000000000000000000", "1200720432259356"},
	} {
		target := new(big.Int).Sqrt(new(big.Int).Mul(integer(tc.target), power2(192)))
		got, err := calculateStep(q, target, l, amount, 600, tc.input, false)
		if err != nil || got.in.String() != tc.in || got.out.String() != tc.out || got.fee.String() != tc.fee {
			t.Fatalf("step=%+v err=%v", got, err)
		}
	}
	got, err := calculateStep(integer("417332158212080721273783715441582"), integer("1452870262520218020823638996"), integer("159344665391607089467575320103"), big.NewInt(1), 1, false, true)
	if err != nil || got.in.Int64() != 1 || got.out.Int64() != 1 || got.fee.Int64() != 1 || got.price.String() != "417332158212080721273783715441581" {
		t.Fatalf("rounding step=%+v err=%v", got, err)
	}
}
func TestExactOutputRequiresSufficientInputBothDirections(t *testing.T) {
	for _, zero := range []bool{true, false} {
		p := power2(96)
		target := sqrtAtTick(600)
		if zero {
			target = sqrtAtTick(-600)
		}
		output := big.NewInt(123456789)
		liquidity := integer("1000000000000000000")
		required, err := calculateStep(p, target, liquidity, output, 3000, false, zero)
		if err != nil {
			t.Fatal(err)
		}
		supplied, err := calculateStep(p, target, liquidity, new(big.Int).Add(required.in, required.fee), 3000, true, zero)
		if err != nil {
			t.Fatal(err)
		}
		if supplied.out.Cmp(output) < 0 {
			t.Fatal("exact output underfunded")
		}
	}
}
