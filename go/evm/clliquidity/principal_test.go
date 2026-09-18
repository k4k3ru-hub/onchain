package clliquidity

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"
)

func number(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("invalid test fixture")
	}
	return v
}
func fixture(t *testing.T, name string) (State, []string) {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Spacing int32               `json:"spacing"`
		Tick    int32               `json:"tick"`
		Price   string              `json:"sqrt_price_x96"`
		Active  string              `json:"active_liquidity"`
		Ticks   [][]json.RawMessage `json:"ticks"`
		Amounts []string            `json:"raw_amounts"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	s := State{SqrtPriceX96: number(f.Price), Tick: f.Tick, Spacing: f.Spacing, ActiveLiquidity: number(f.Active), Complete: true}
	for _, r := range f.Ticks {
		var i int32
		var g, n string
		for j, d := range []any{&i, &g, &n} {
			if err := json.Unmarshal(r[j], d); err != nil {
				t.Fatal(err)
			}
		}
		s.Ticks = append(s.Ticks, Tick{i, number(g), number(n)})
	}
	return s, f.Amounts
}
func TestRecordedPrincipal(t *testing.T) {
	for _, name := range []string{"base-v3", "base-aerodrome-live", "base-v4", "robinhood-v4-verified"} {
		t.Run(name, func(t *testing.T) {
			s, want := fixture(t, name)
			before, _ := json.Marshal(s)
			got, err := Calculate(s)
			if err != nil {
				t.Fatal(err)
			}
			if got.Token0.String() != want[0] || got.Token1.String() != want[1] {
				t.Fatalf("amounts=%v want=%v", got, want)
			}
			after, _ := json.Marshal(s)
			if string(before) != string(after) {
				t.Fatal("inputs mutated")
			}
		})
	}
}
func TestRejectIncompleteAndInconsistentState(t *testing.T) {
	cases := []struct {
		name string
		edit func(*State)
	}{
		{"incomplete", func(s *State) { s.Complete = false }},
		{"missing upper", func(s *State) { s.Ticks = s.Ticks[:1] }},
		{"active mismatch", func(s *State) { s.ActiveLiquidity.Add(s.ActiveLiquidity, big.NewInt(1)) }},
		{"negative gross", func(s *State) { s.Ticks[0].Gross.Neg(s.Ticks[0].Gross) }},
		{"order", func(s *State) { s.Ticks[0], s.Ticks[1] = s.Ticks[1], s.Ticks[0] }},
		{"price mismatch", func(s *State) { s.Tick = 0 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := fixture(t, "base-v4")
			c.edit(&s)
			a, err := Calculate(s)
			if err == nil || a.Token0 != nil {
				t.Fatalf("partial result: %v %v", a, err)
			}
		})
	}
}
func TestRangesAndBoundary(t *testing.T) {
	s := State{Spacing: 10, Complete: true, SqrtPriceX96: sqrtAtTick(0), Tick: 0, ActiveLiquidity: big.NewInt(1000000), Ticks: []Tick{{-10, big.NewInt(1000000), big.NewInt(1000000)}, {10, big.NewInt(1000000), big.NewInt(-1000000)}}}
	a, err := Calculate(s)
	if err != nil || a.Token0.Sign() <= 0 || a.Token1.Sign() <= 0 {
		t.Fatalf("inside: %v %v", a, err)
	}
	s.Tick = -20
	s.SqrtPriceX96 = sqrtAtTick(-20)
	s.ActiveLiquidity = big.NewInt(0)
	a, err = Calculate(s)
	if err != nil || a.Token0.Sign() <= 0 || a.Token1.Sign() != 0 {
		t.Fatalf("below: %v %v", a, err)
	}
	s.Tick = 20
	s.SqrtPriceX96 = sqrtAtTick(20)
	a, err = Calculate(s)
	if err != nil || a.Token0.Sign() != 0 || a.Token1.Sign() <= 0 {
		t.Fatalf("above: %v %v", a, err)
	}
	s.Tick = -11
	s.SqrtPriceX96 = sqrtAtTick(-10)
	if _, err = Calculate(s); err != nil {
		t.Fatalf("exact crossing: %v", err)
	}
	s.Ticks = nil
	s.Tick = 0
	s.SqrtPriceX96 = sqrtAtTick(0)
	a, err = Calculate(s)
	if err != nil || a.Token0.Sign() != 0 || a.Token1.Sign() != 0 {
		t.Fatalf("empty: %v %v", a, err)
	}
}
func TestUSDCValuation(t *testing.T) {
	s, _ := fixture(t, "base-v3")
	a, err := Calculate(s)
	if err != nil {
		t.Fatal(err)
	}
	price, err := PriceRatio(s.SqrtPriceX96, 18, 6)
	if err != nil {
		t.Fatal(err)
	}
	value, err := Value(a, 18, 6, price, big.NewRat(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if value.FloatString(2) != "8786871.68" {
		t.Fatal(value.FloatString(2))
	}
	if _, err := Value(a, 18, 6, nil, big.NewRat(1, 1)); err == nil {
		t.Fatal("unknown price accepted")
	}
}
