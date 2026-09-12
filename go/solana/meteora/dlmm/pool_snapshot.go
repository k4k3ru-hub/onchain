package dlmm

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
	out := &solana.PoolStateSnapshot{Protocol: "dlmm", Components: map[string]solana.PoolStateComponent{}}

	out.Token0, out.Token1 = pool.TokenXMint, pool.TokenYMint
	// Compute (1 + binStep/10000)^activeID at 256-bit precision, bounded to the protocol price domain.
	if pool.BinStep == 0 || pool.ActiveBinID < -443636 || pool.ActiveBinID > 443636 {
		return nil, fmt.Errorf("failed to read pool snapshot: bin_price=out_of_range")
	}
	base := new(big.Float).SetPrec(256).SetRat(new(big.Rat).SetFrac64(10000+int64(pool.BinStep), 10000))
	price := new(big.Float).SetPrec(256).SetInt64(1)
	exponent := int64(pool.ActiveBinID)
	if exponent < 0 {
		exponent = -exponent
	}
	for exponent > 0 {
		if exponent&1 != 0 {
			price.Mul(price, base)
		}
		exponent >>= 1
		if exponent > 0 {
			base.Mul(base, base)
		}
	}
	if pool.ActiveBinID < 0 {
		price.Quo(new(big.Float).SetPrec(256).SetInt64(1), price)
	}
	if price.IsInf() || price.MantExp(nil) > 128 || price.MantExp(nil) < -128 {
		return nil, fmt.Errorf("failed to read pool snapshot: bin_price=out_of_range")
	}
	ratio, _ := price.Rat(nil)
	out.RawPrice = ratio.RatString()
	out.Components["pool"] = solana.PoolComponent(retained, map[string]string{"active_bin": fmt.Sprint(pool.ActiveBinID), "bin_step": fmt.Sprint(pool.BinStep)})
	out.PriceInputs = []solana.PoolStateComponent{out.Components["pool"]}
	out.CoverageUnit = "bin"
	for address, account := range s.accounts.Accounts {
		if account.Account == nil || account.Account.Owner != s.programID || !bytes.HasPrefix(account.Account.Data, binArrayDiscriminator[:]) {
			continue
		}
		array, err := decodeBinArray(address, account.Account.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to read pool snapshot: %w", err)
		}
		if array.Pool != s.pool {
			continue
		}
		lower := array.Index * binArraySize
		out.Coverage = append(out.Coverage, solana.PoolCoverageSegment{Lower: lower, Upper: lower + binArraySize - 1})
	}

	sort.Slice(out.Coverage, func(i, j int) bool { return out.Coverage[i].Lower < out.Coverage[j].Lower })
	return out, nil
}
