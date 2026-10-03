package clmm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

func fundedExactOutput(t *testing.T, native bool) SwapTransactionParams {
	t.Helper()
	p := fundedSwap(t, native)
	p.MaximumAmountIn, p.AmountOut = p.AmountIn, p.MinimumAmountOut
	p.AmountIn, p.MinimumAmountOut = 0, 0
	return p
}

// TestFundedSwapExactOutput verifies the signed funding cap, protocol mode,
// exact output guard, and separate refund and output recipients in both directions.
//
// Version:
//   - 2026-10-02: Added.
func TestFundedSwapExactOutput(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native_input=%t", native), func(t *testing.T) {
			p := fundedExactOutput(t, native)
			built, err := BuildSwapTransaction(testDeployment(), p)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := built.MarshalBCS()
			if err != nil {
				t.Fatal(err)
			}
			tx, err := sui.ParseTransactionData(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if tx.Sender != p.Sender || tx.GasData.Owner != p.Sender || tx.GasData.Budget != p.GasBudget || tx.GasData.Price != p.GasPrice || tx.ExpirationEpoch == nil || *tx.ExpirationEpoch != p.ExpirationEpoch {
				t.Fatal("sender, gas or expiration changed")
			}
			pure := func(a sui.Argument) []byte {
				t.Helper()
				if a.Kind != sui.ArgumentKindInput || int(a.Index) >= len(tx.Transaction.Inputs) || tx.Transaction.Inputs[a.Index].Kind != sui.InputKindPure {
					t.Fatal("expected signed pure input")
				}
				return tx.Transaction.Inputs[a.Index].Pure
			}
			number := func(a sui.Argument) uint64 {
				t.Helper()
				v := pure(a)
				if len(v) != 8 {
					t.Fatal("expected u64 input")
				}
				return binary.LittleEndian.Uint64(v)
			}
			commands := tx.Transaction.Commands
			splitIndex := 0
			if !native {
				if commands[0].MergeCoins == nil || len(commands[0].MergeCoins.Sources) != 1 {
					t.Fatal("input consolidation missing")
				}
				splitIndex++
			}
			split := commands[splitIndex].SplitCoins
			if split == nil || len(split.Amounts) != 1 || (split.Coin.Kind == sui.ArgumentKindGas) != native || number(split.Amounts[0]) != p.MaximumAmountIn {
				t.Fatal("swap funding is not capped independently of the output target")
			}
			found := false
			for _, command := range commands {
				call := command.MoveCall
				if call == nil || (call.Function != "flash_swap") {
					continue
				}
				if found {
					t.Fatal("multiple swaps")
				}
				found = true
				if !bytes.Equal(pure(call.Arguments[3]), []byte{0}) || number(call.Arguments[4]) != p.AmountOut {
					t.Fatal("protocol call does not fix the output quantity")
				}
				direction := byte(0)
				if p.A2B {
					direction = 1
				}
				if !bytes.Equal(pure(call.Arguments[2]), []byte{direction}) {
					t.Fatal("swap direction changed")
				}
			}
			if !found {
				t.Fatal("swap missing")
			}
			if len(commands) < 6 {
				t.Fatal("refund or exact output guard missing")
			}
			tail := commands[len(commands)-6:]
			for i, function := range []string{"from_balance", "public_transfer", "split", "destroy_zero", "from_balance", "public_transfer"} {
				call := tail[i].MoveCall
				if call == nil || call.Package.String() != "0x0000000000000000000000000000000000000000000000000000000000000002" || call.Function != function {
					t.Fatalf("missing framework guard/refund command: %s", function)
				}
			}
			if !bytes.Equal(pure(tail[1].MoveCall.Arguments[1]), p.Sender.Bytes()) || !bytes.Equal(pure(tail[5].MoveCall.Arguments[1]), p.Recipient.Bytes()) {
				t.Fatal("input refund must go to the sender and output to the recipient")
			}
			guard := tail[2].MoveCall
			if number(guard.Arguments[1]) != p.AmountOut {
				t.Fatal("output target guard changed")
			}
			if guard.Arguments[0] != tail[3].MoveCall.Arguments[0] || guard.TypeArguments[0] != tail[3].MoveCall.TypeArguments[0] {
				t.Fatal("unexpected output excess is not rejected")
			}
			guardResult := sui.Argument{Kind: sui.ArgumentKindResult, Index: uint16(len(commands) - 4)}
			if tail[4].MoveCall.Arguments[0] != guardResult {
				t.Fatal("recipient does not receive the exact guarded quantity")
			}
			if tail[0].MoveCall.TypeArguments[0] == tail[4].MoveCall.TypeArguments[0] {
				t.Fatal("input refund and output types must differ")
			}
		})
	}
}

// TestFundedSwapRejectsInvalidExactOutput rejects ambiguous sizing and funding
// that cannot cover the maximum input independently of the reserved gas.
//
// Version:
//   - 2026-10-02: Added.
func TestFundedSwapRejectsInvalidExactOutput(t *testing.T) {
	cases := map[string]func(*SwapTransactionParams){
		"missing output":       func(p *SwapTransactionParams) { p.AmountOut = 0 },
		"missing payment cap":  func(p *SwapTransactionParams) { p.MaximumAmountIn = 0 },
		"mixed input amount":   func(p *SwapTransactionParams) { p.AmountIn = 1 },
		"mixed output minimum": func(p *SwapTransactionParams) { p.MinimumAmountOut = 1 },
		"cap exceeds coins":    func(p *SwapTransactionParams) { p.MaximumAmountIn = 1401 },
		"wrong owner":          func(p *SwapTransactionParams) { p.InputCoins[0].Owner = p.Recipient },
		"wrong input asset":    func(p *SwapTransactionParams) { p.InputCoins[0].CoinType = "0x3::usdc::Other" },
		"duplicate coins":      func(p *SwapTransactionParams) { p.InputCoins[1] = p.InputCoins[0] },
		"overlapping gas":      func(p *SwapTransactionParams) { p.InputCoins[0].Reference = p.GasCoins[0].Reference },
		"missing gas":          func(p *SwapTransactionParams) { p.GasCoins = nil },
		"coin overflow":        func(p *SwapTransactionParams) { p.InputCoins[1].Balance = ^uint64(0) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := fundedExactOutput(t, false)
			change(&p)
			if _, err := BuildSwapTransaction(testDeployment(), p); err == nil {
				t.Fatal("accepted invalid exact-output funding")
			}
		})
	}
	for _, change := range []func(*SwapTransactionParams){
		func(p *SwapTransactionParams) { p.MaximumAmountIn++ },
		func(p *SwapTransactionParams) { p.MaximumAmountIn = ^uint64(0) },
		func(p *SwapTransactionParams) { p.GasCoins[0].Balance = p.GasBudget - 1 },
		func(p *SwapTransactionParams) { p.InputCoins = append(p.InputCoins, p.GasCoins[0]) },
	} {
		p := fundedExactOutput(t, true)
		change(&p)
		if _, err := BuildSwapTransaction(testDeployment(), p); err == nil {
			t.Fatal("accepted native input that spends reserved gas or aliases gas coins")
		}
	}
}
