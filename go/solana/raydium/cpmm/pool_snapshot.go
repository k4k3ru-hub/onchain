package cpmm

import (
	"fmt"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"math/big"
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
	out := &solana.PoolStateSnapshot{Protocol: "cpmm", Components: map[string]solana.PoolStateComponent{}}

	out.Token0, out.Token1 = pool.Token0Mint, pool.Token1Mint
	out.Components["pool"] = solana.PoolComponent(retained, map[string]string{"protocol_fees_0": fmt.Sprint(pool.ProtocolFees0), "fund_fees_0": fmt.Sprint(pool.FundFees0), "creator_fees_0": fmt.Sprint(pool.CreatorFees0), "protocol_fees_1": fmt.Sprint(pool.ProtocolFees1), "fund_fees_1": fmt.Sprint(pool.FundFees1), "creator_fees_1": fmt.Sprint(pool.CreatorFees1)})
	reserves := [2]uint64{}
	for i, address := range []solana.Address{pool.Token0Vault, pool.Token1Vault} {
		a := s.accounts.Accounts[address]
		amount, err := decodeTokenAmount(a.Account)
		if err != nil {
			return nil, fmt.Errorf("failed to read pool snapshot: %w", err)
		}
		fees, ok := checkedSum(pool.ProtocolFees0, pool.FundFees0, pool.CreatorFees0)
		if i == 1 {
			fees, ok = checkedSum(pool.ProtocolFees1, pool.FundFees1, pool.CreatorFees1)
		}
		if !ok || amount <= fees {
			return nil, fmt.Errorf("failed to read pool snapshot: reserves=invalid")
		}
		reserves[i] = amount - fees
		out.Components[fmt.Sprintf("vault_%d", i)] = solana.PoolComponent(a, map[string]string{"balance": fmt.Sprint(amount), "effective_reserve": fmt.Sprint(reserves[i])})
	}
	out.PriceInputs = []solana.PoolStateComponent{out.Components["pool"], out.Components["vault_0"], out.Components["vault_1"]}
	out.RawPrice = new(big.Rat).SetFrac(new(big.Int).SetUint64(reserves[1]), new(big.Int).SetUint64(reserves[0])).RatString()
	out.CoverageUnit = "reserves"

	return out, nil
}
