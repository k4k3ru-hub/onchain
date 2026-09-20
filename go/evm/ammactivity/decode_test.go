package ammactivity

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func eventLog(topic common.Hash, topics, words int) types.Log {
	l := types.Log{Topics: make([]common.Hash, topics), Data: make([]byte, words*32)}
	l.Topics[0] = topic
	return l
}
func setWord(l *types.Log, offset int, n *big.Int) {
	v := new(big.Int).Set(n)
	if v.Sign() < 0 {
		v.Add(v, new(big.Int).Lsh(big.NewInt(1), 256))
	}
	v.FillBytes(l.Data[offset*32 : (offset+1)*32])
}

// TestDecodeSwapDirections verifies venue sign conventions, one-sided fee swaps and integer precision.
//
// Version:
//   - 2026-09-21: Added.
func TestDecodeSwapDirections(t *testing.T) {
	large := new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
	for _, venue := range []string{"uniswap-v3", "aerodrome", "uniswap-v4"} {
		for _, reverse := range []bool{false, true} {
			l := eventLog(swapV3, 3, 5)
			if venue == "uniswap-v4" {
				l = eventLog(swapV4, 3, 6)
			}
			a, b := new(big.Int).Set(large), big.NewInt(-7)
			want := "token0_to_token1"
			if reverse {
				a, b = big.NewInt(-7), new(big.Int).Set(large)
				want = "token1_to_token0"
			}
			if venue == "uniswap-v4" {
				a.Neg(a)
				b.Neg(b)
			}
			setWord(&l, 0, a)
			setWord(&l, 1, b)
			e, ok, err := Decode(venue, l)
			if err != nil || !ok || e.Kind != "swap" || e.Direction != want {
				t.Fatalf("%s reverse=%t: %+v %v", venue, reverse, e, err)
			}
			if e.Amount0.Cmp(new(big.Int).Abs(a)) != 0 || e.Amount1.Cmp(new(big.Int).Abs(b)) != 0 {
				t.Fatal("amount rounded")
			}
		}
	}
	l := eventLog(swapV3, 3, 5)
	setWord(&l, 0, big.NewInt(1))
	if e, _, err := Decode("uniswap-v3", l); err != nil || e.Direction != "token0_to_token1" {
		t.Fatal(e, err)
	}
}

// TestDecodeLiquidity distinguishes principal changes from pokes, fee collection and donations.
//
// Version:
//   - 2026-09-21: Added.
func TestDecodeLiquidity(t *testing.T) {
	for _, tc := range []struct {
		topic         common.Hash
		words, offset int
		kind          string
	}{{mint, 4, 1, "added"}, {burn, 3, 0, "removed"}} {
		l := eventLog(tc.topic, 4, tc.words)
		setWord(&l, tc.offset, big.NewInt(100))
		setWord(&l, tc.offset+1, big.NewInt(13))
		setWord(&l, tc.offset+2, big.NewInt(17))
		e, ok, err := Decode("aerodrome", l)
		if err != nil || !ok || e.Kind != tc.kind || e.Amount0.String() != "13" || e.Amount1.String() != "17" {
			t.Fatal(e, err)
		}
		setWord(&l, tc.offset, new(big.Int))
		if _, ok, err := Decode("uniswap-v3", l); ok || err != nil {
			t.Fatal("zero poke counted", err)
		}
	}
	l := eventLog(modify, 3, 4)
	setWord(&l, 2, big.NewInt(-100))
	e, ok, err := Decode("uniswap-v4", l)
	if err != nil || !ok || e.Kind != "removed" || e.Amount0 != nil || e.Amount1 != nil {
		t.Fatal(e, err)
	}
	for _, sig := range []string{"Collect(address,address,int24,int24,uint128,uint128)", "Donate(bytes32,address,uint256,uint256)"} {
		if _, ok, err := Decode("uniswap-v4", eventLog(signature(sig), 3, 2)); ok || err != nil {
			t.Fatal("fees counted", err)
		}
	}
	l = eventLog(swapV4, 3, 6)
	setWord(&l, 0, new(big.Int).Lsh(big.NewInt(1), 128))
	if _, _, err := Decode("uniswap-v4", l); err == nil {
		t.Fatal("int128 overflow accepted")
	}
	l.Data = l.Data[:31]
	if _, _, err := Decode("uniswap-v4", l); err == nil {
		t.Fatal("malformed log accepted")
	}
}
