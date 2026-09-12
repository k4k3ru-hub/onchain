package clmm

import (
	"bytes"
	"fmt"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"math/big"
	"sort"
)

// PoolSnapshot derives the pool mid and loaded coverage exclusively from frozen accounts.
// Price provenance uses only accounts contributing to price, never unrelated account refreshes.
//
// Version:
//   - 2026-09-12: Added.
func (s *QuoteSnapshot) PoolSnapshot() (*solana.PoolStateSnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("failed to read pool snapshot: snapshot=null")
	}
	retained := s.accounts.Accounts[s.pool]
	if retained.Account == nil || retained.Account.Owner != s.programID {
		return nil, fmt.Errorf("failed to read pool snapshot: pool_account=invalid")
	}
	pool, err := decodePool(s.pool, retained.Account.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to read pool snapshot: %w", err)
	}
	out := &solana.PoolStateSnapshot{Protocol: "clmm", Components: map[string]solana.PoolStateComponent{}}

	out.Token0, out.Token1 = pool.Token0Mint, pool.Token1Mint
	value := pool.SqrtPriceX64
	for i, j := 0, len(value)-1; i < j; i, j = i+1, j-1 {
		value[i], value[j] = value[j], value[i]
	}
	sqrt := new(big.Int).SetBytes(value[:])
	if sqrt.Sign() <= 0 {
		return nil, fmt.Errorf("failed to read pool snapshot: sqrt_price=invalid")
	}
	out.RawPrice = new(big.Rat).SetFrac(new(big.Int).Mul(sqrt, sqrt), new(big.Int).Lsh(big.NewInt(1), 128)).RatString()
	liquidity := pool.Liquidity
	for i, j := 0, len(liquidity)-1; i < j; i, j = i+1, j-1 {
		liquidity[i], liquidity[j] = liquidity[j], liquidity[i]
	}
	out.Components["pool"] = solana.PoolComponent(retained, map[string]string{"sqrt_price": sqrt.String(), "sqrt_price_fractional_bits": "64", "tick": fmt.Sprint(pool.CurrentTick), "tick_spacing": fmt.Sprint(pool.TickSpacing), "liquidity": new(big.Int).SetBytes(liquidity[:]).String()})
	out.PriceInputs = []solana.PoolStateComponent{out.Components["pool"]}
	out.CoverageUnit = "tick"
	for address, account := range s.accounts.Accounts {
		if account.Account == nil || account.Account.Owner != s.programID || !bytes.HasPrefix(account.Account.Data, tickArrayDiscriminator[:]) {
			continue
		}
		array, err := decodeTickArray(address, account.Account.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to read pool snapshot: %w", err)
		}
		if array.Pool != s.pool {
			continue
		}
		lower := int64(array.StartTickIndex)
		out.Coverage = append(out.Coverage, solana.PoolCoverageSegment{Lower: lower, Upper: lower + int64(pool.TickSpacing)*tickArraySize - 1})
	}

	sort.Slice(out.Coverage, func(i, j int) bool { return out.Coverage[i].Lower < out.Coverage[j].Lower })
	return out, nil
}
