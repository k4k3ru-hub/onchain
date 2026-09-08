package clmm

import (
	"encoding/json"
	sui "github.com/k4k3ru-hub/onchain/go/sui"
	"os"
	"strconv"
	"testing"
)

// TestOfficialLocalQuoteVectors compares both directions and amount modes with official SDK outputs.
//
// Version:
//   - 2026-09-08: Added.
func TestOfficialLocalQuoteVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/local_quote_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Pool    json.RawMessage
		Ticks   []json.RawMessage
		Vectors []struct {
			A2B     bool
			Input   bool
			Amount  string
			In      string
			Out     string
			Fee     string
			After   string
			Crosses int
		}
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	p, err := ParsePool(&sui.Object{Move: &sui.MoveObject{Type: "0x1::pool::Pool<0x2::a::A,0x2::b::B>", JSON: f.Pool}})
	if err != nil {
		t.Fatal(err)
	}
	s := LocalSnapshot{Pool: *p}
	for _, v := range f.Ticks {
		tick, e := parseTick(v)
		if e != nil {
			t.Fatal(e)
		}
		s.Ticks = append(s.Ticks, tick)
	}
	crossed := false
	for _, v := range f.Vectors {
		amount, e := strconv.ParseUint(v.Amount, 10, 64)
		if e != nil {
			t.Fatal(e)
		}
		r, e := s.Quote(amount, v.A2B, v.Input)
		if e != nil {
			t.Fatal(e)
		}
		if strconv.FormatUint(r.AmountIn+r.FeeAmount, 10) != v.In || strconv.FormatUint(r.AmountOut, 10) != v.Out || strconv.FormatUint(r.FeeAmount, 10) != v.Fee || r.AfterSqrtPrice.String() != v.After {
			t.Fatalf("vector %+v got %+v", v, r)
		}
		crossed = crossed || v.Crosses > 1
	}
	if len(f.Vectors) < 12 || !crossed {
		t.Fatal("insufficient vector coverage")
	}
}
