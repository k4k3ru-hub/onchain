package clmm

import (
	"bytes"
	"encoding/binary"
	"testing"

	sui "github.com/k4k3ru-hub/onchain/go/sui"
)

func fundedSwap(t *testing.T, native bool) SwapTransactionParams {
	t.Helper()
	address := func(value string) sui.Address {
		a, err := sui.ParseAddress(value)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	owner := address("0x11")
	coin := func(id, coinType string, balance uint64) sui.Coin {
		return sui.Coin{Owner: owner, CoinType: coinType, Balance: balance, Reference: sui.ObjectReference{Address: address(id), Version: 123, Digest: sui.ObjectDigest{1}}}
	}
	p := SwapTransactionParams{Pool: Pool{Address: address("0x99"), InitialVersion: 12, CoinTypeA: "0x3::usdc::USDC", CoinTypeB: "0x2::sui::SUI"}, Sender: owner, Recipient: address("0x12"), A2B: !native, AmountIn: 1000, MinimumAmountOut: 2000, GasPrice: 1000, GasBudget: 50000000, ExpirationEpoch: 1232, GasCoins: []sui.Coin{coin("0x81", "0x2::sui::SUI", 50001000)}}
	if !native {
		p.InputCoins = []sui.Coin{coin("0x91", "0x3::usdc::USDC", 400), coin("0x92", "0x3::usdc::USDC", 1000)}
	}
	return p
}

// TestFundedSwapTransaction verifies funding, full-fill and output guards in both directions.
//
// Version:
//   - 2026-09-24: Added.
func TestFundedSwapTransaction(t *testing.T) {
	for _, native := range []bool{true, false} {
		p := fundedSwap(t, native)
		tx, err := BuildSwapTransaction(testDeployment(), p)
		if err != nil {
			t.Fatal(err)
		}
		data, err := tx.MarshalBCS()
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := sui.ParseTransactionData(data)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Sender != p.Sender || decoded.GasData.Owner != p.Sender || *decoded.ExpirationEpoch != p.ExpirationEpoch {
			t.Fatal("transaction identities or expiration changed")
		}
		commands := decoded.Transaction.Commands
		start := 0
		if !native {
			if commands[0].MergeCoins == nil || len(commands[0].MergeCoins.Sources) != 1 {
				t.Fatal("input merge missing")
			}
			start++
		}
		split := commands[start].SplitCoins
		if split == nil || (split.Coin.Kind == sui.ArgumentKindGas) != native {
			t.Fatal("incorrect funding source")
		}
		if binary.LittleEndian.Uint64(decoded.Transaction.Inputs[split.Amounts[0].Index].Pure) != p.AmountIn {
			t.Fatal("incorrect input cap")
		}
		last := commands[len(commands)-5:]
		for i, name := range []string{"destroy_zero", "split", "join", "from_balance", "public_transfer"} {
			if last[i].MoveCall == nil || last[i].MoveCall.Function != name {
				t.Fatalf("guard/cleanup missing: %s", name)
			}
		}
		minimum := last[1].MoveCall.Arguments[1]
		if binary.LittleEndian.Uint64(decoded.Transaction.Inputs[minimum.Index].Pure) != p.MinimumAmountOut {
			t.Fatal("incorrect output guard")
		}
		to := last[4].MoveCall.Arguments[1]
		if !bytes.Equal(decoded.Transaction.Inputs[to.Index].Pure, p.Recipient.Bytes()) {
			t.Fatal("incorrect recipient")
		}
	}
}

// TestFundedSwapRejectsInvalidCoins verifies ownership, balance, duplicate and gas isolation checks.
//
// Version:
//   - 2026-09-24: Added.
func TestFundedSwapRejectsInvalidCoins(t *testing.T) {
	cases := map[string]func(*SwapTransactionParams){
		"insufficient gas":   func(p *SwapTransactionParams) { p.GasCoins[0].Balance = p.GasBudget - 1 },
		"wrong owner":        func(p *SwapTransactionParams) { p.GasCoins[0].Owner = p.Recipient },
		"wrong type":         func(p *SwapTransactionParams) { p.InputCoins[0].CoinType = "0x3::usdc::Other" },
		"insufficient input": func(p *SwapTransactionParams) { p.InputCoins[1].Balance = 0 },
		"duplicate inputs":   func(p *SwapTransactionParams) { p.InputCoins[1] = p.InputCoins[0] },
		"gas overlap":        func(p *SwapTransactionParams) { p.InputCoins[0].Reference = p.GasCoins[0].Reference },
		"overflow":           func(p *SwapTransactionParams) { p.InputCoins[1].Balance = ^uint64(0) },
		"empty input":        func(p *SwapTransactionParams) { p.InputCoins = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := fundedSwap(t, false)
			change(&p)
			if _, err := BuildSwapTransaction(testDeployment(), p); err == nil {
				t.Fatal("accepted invalid funds")
			}
		})
	}
	p := fundedSwap(t, true)
	p.GasCoins[0].Balance--
	if _, err := BuildSwapTransaction(testDeployment(), p); err == nil {
		t.Fatal("spent reserved gas budget")
	}
}
