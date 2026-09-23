package lpprotection

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

func covered(state clliquidity.State, positions []Position) bool {
	counts := make(map[int32]*clliquidity.Tick)
	for _, p := range positions {
		for _, tick := range []int32{p.Lower, p.Upper} {
			if counts[tick] == nil {
				counts[tick] = &clliquidity.Tick{Index: tick, Gross: new(big.Int), Net: new(big.Int)}
			}
			counts[tick].Gross.Add(counts[tick].Gross, p.Liquidity)
			if tick == p.Lower {
				counts[tick].Net.Add(counts[tick].Net, p.Liquidity)
			} else {
				counts[tick].Net.Sub(counts[tick].Net, p.Liquidity)
			}
		}
	}
	if len(counts) != len(state.Ticks) {
		return false
	}
	for _, tick := range state.Ticks {
		p := counts[tick.Index]
		if p == nil || p.Gross.Cmp(tick.Gross) != 0 || p.Net.Cmp(tick.Net) != 0 {
			return false
		}
	}
	return true
}

func aggregate(price *big.Int, positions []Position) (*Observation, string, error) {
	total := [2]*big.Rat{new(big.Rat), new(big.Rat)}
	locked := [2]*big.Rat{new(big.Rat), new(big.Rat)}
	result := &Observation{AllPositionsProtected: true, CanWeakenProtection: flag(false)}
	weakUnknown := false
	for _, p := range positions {
		if p.Kind != "locked" && p.Kind != "withdrawable" {
			return nil, "unsupported_custody", nil
		}
		a, b, err := clliquidity.PositionPrincipal(price, p.Liquidity, p.Lower, p.Upper)
		if err != nil {
			return nil, "", fmt.Errorf("failed to calculate lp protection: %w", err)
		}
		for i, amount := range []*big.Rat{a, b} {
			total[i].Add(total[i], amount)
			if p.Kind == "locked" {
				locked[i].Add(locked[i], amount)
			}
		}
		if p.Kind == "withdrawable" {
			result.AllPositionsProtected = false
		} else {
			if p.UnlockAt == nil {
				return nil, "", fmt.Errorf("failed to calculate lp protection: unlock_at=null")
			}
			if result.EarliestUnlockAt == nil || p.UnlockAt.Before(*result.EarliestUnlockAt) {
				t := *p.UnlockAt
				result.EarliestUnlockAt = &t
			}
		}
		if p.CanWeakenProtection == nil {
			weakUnknown = true
		} else if *p.CanWeakenProtection {
			result.CanWeakenProtection = flag(true)
		}
	}
	if total[0].Sign() == 0 && total[1].Sign() == 0 {
		return nil, "no_principal", nil
	}
	if !*result.CanWeakenProtection && weakUnknown {
		result.CanWeakenProtection = nil
	}
	for i, target := range []*TokenProtection{&result.Token0, &result.Token1} {
		if total[i].Sign() == 0 {
			continue
		}
		v := percentage(locked[i], total[i])
		zero := "0"
		target.LockedPercentage = &v
		target.PermanentlyProtectedPercentage = &zero
	}
	return result, "", nil
}

func percentage(n, d *big.Rat) string {
	r := new(big.Rat).Quo(n, d)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	scaled := new(big.Int).Mul(r.Num(), new(big.Int).Mul(big.NewInt(100), scale))
	scaled.Quo(scaled, r.Denom())
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(scaled, scale, fraction)
	if fraction.Sign() == 0 {
		return whole.String()
	}
	decimal := fraction.String()
	decimal = strings.Repeat("0", 18-len(decimal)) + decimal
	return whole.String() + "." + strings.TrimRight(decimal, "0")
}
