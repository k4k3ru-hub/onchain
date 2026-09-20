// Package ammactivity decodes AMM activity without making RPC requests.
package ammactivity

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type Event struct {
	Kind             string   // swap, added, removed
	Direction        string   // token0_to_token1, token1_to_token0, or empty for a zero swap
	Amount0, Amount1 *big.Int // absolute raw token quantities; nil when not emitted
}

var (
	swapV3 = signature("Swap(address,address,int256,int256,uint160,uint128,int24)")
	swapV4 = signature("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)")
	mint   = signature("Mint(address,address,int24,int24,uint128,uint256,uint256)")
	burn   = signature("Burn(address,int24,int24,uint128,uint256,uint256)")
	modify = signature("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)")
)

func signature(s string) common.Hash { return crypto.Keccak256Hash([]byte(s)) }

// Decode extracts pool-level quantities for Uniswap V3, V4 and Aerodrome Slipstream.
// Callers validate emitter/pool identity and own canonicality and deduplication.
// Collect, Donate and zero liquidity pokes do not change LP principal.
// V4 Swap deltas use the caller sign convention, before afterSwap hook adjustments.
//
// Returns:
//   - A decoded event and true, or false for an unrelated/no-op event.
//
// Version:
//   - 2026-09-21: Added.
func Decode(venue string, log types.Log) (Event, bool, error) {
	fail := func() (Event, bool, error) {
		return Event{}, false, fmt.Errorf("failed to decode amm activity: log=invalid")
	}
	if venue != "uniswap-v3" && venue != "uniswap-v4" && venue != "aerodrome" {
		return Event{}, false, fmt.Errorf("failed to decode amm activity: venue=invalid")
	}
	if len(log.Topics) == 0 {
		return fail()
	}
	topic := log.Topics[0]
	v4 := venue == "uniswap-v4"
	words, topics := 0, 0
	switch {
	case !v4 && topic == swapV3:
		words, topics = 5, 3
	case v4 && topic == swapV4:
		words, topics = 6, 3
	case !v4 && topic == mint:
		words, topics = 4, 4
	case !v4 && topic == burn:
		words, topics = 3, 4
	case v4 && topic == modify:
		words, topics = 4, 3
	default:
		return Event{}, false, nil
	}
	if len(log.Topics) != topics || len(log.Data) != words*32 {
		return fail()
	}
	word := func(i int) *big.Int { return new(big.Int).SetBytes(log.Data[i*32 : (i+1)*32]) }
	signed := func(i, bits int) (*big.Int, bool) {
		n := word(i)
		if log.Data[i*32]&128 != 0 {
			n.Sub(n, new(big.Int).Lsh(big.NewInt(1), 256))
		}
		limit := new(big.Int).Lsh(big.NewInt(1), uint(bits-1))
		return n, n.Cmp(new(big.Int).Neg(limit)) >= 0 && n.Cmp(limit) < 0
	}
	if topic == swapV3 || topic == swapV4 {
		bits := 256
		if v4 {
			bits = 128
		}
		a, ok := signed(0, bits)
		b, valid := signed(1, bits)
		if !ok || !valid {
			return fail()
		}
		if v4 {
			a.Neg(a)
			b.Neg(b)
		}
		direction := ""
		switch {
		case a.Sign() >= 0 && b.Sign() <= 0 && (a.Sign() > 0 || b.Sign() < 0):
			direction = "token0_to_token1"
		case a.Sign() <= 0 && b.Sign() >= 0 && (a.Sign() < 0 || b.Sign() > 0):
			direction = "token1_to_token0"
		case a.Sign() != 0 || b.Sign() != 0:
			return fail()
		}
		return Event{Kind: "swap", Direction: direction, Amount0: a.Abs(a), Amount1: b.Abs(b)}, true, nil
	}
	if topic == modify {
		delta, _ := signed(2, 256)
		if delta.Sign() == 0 {
			return Event{}, false, nil
		}
		kind := "added"
		if delta.Sign() < 0 {
			kind = "removed"
		}
		return Event{Kind: kind}, true, nil
	}
	offset, kind := 0, "removed"
	if topic == mint {
		offset, kind = 1, "added"
	}
	liquidity := word(offset)
	if liquidity.BitLen() > 128 {
		return fail()
	}
	if liquidity.Sign() == 0 {
		return Event{}, false, nil
	}
	return Event{Kind: kind, Amount0: word(offset + 1), Amount1: word(offset + 2)}, true, nil
}
