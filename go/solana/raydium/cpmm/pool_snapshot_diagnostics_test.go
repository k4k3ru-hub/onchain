package cpmm

import (
	"fmt"
	solana "github.com/k4k3ru-hub/onchain/go/solana"
	"strings"
	"testing"
)

// TestPoolSnapshotDiagnostics distinguishes absent accounts from invalid ownership.
//
// Version:
//   - 2026-09-17: Added.
func TestPoolSnapshotDiagnostics(t *testing.T) {
	pool, program := solana.Address{1}, solana.Address{2}
	for _, tc := range []struct {
		name    string
		entries map[solana.Address]solana.RetainedAccount
		want    []string
	}{
		{"missing", nil, []string{"pool account missing", "pool_account=null", "snapshot_slot=40", "account_count=0"}},
		{"null", map[solana.Address]solana.RetainedAccount{pool: {Slot: 42}}, []string{"retained pool account missing", "pool_account=null", "slot=42"}},
		{"wrong_owner", map[solana.Address]solana.RetainedAccount{pool: {Slot: 42, Account: &solana.Account{Address: pool, Owner: solana.Address{3}, Data: []byte{1, 2, 3}}}}, []string{"pool account owner mismatch", "slot=42", "data_length=3", fmt.Sprintf("actual_owner=%q", (solana.Address{3}).String())}},
		{"zero_owner", map[solana.Address]solana.RetainedAccount{pool: {Slot: 43, Account: &solana.Account{Address: pool}}}, []string{"pool account owner mismatch", "slot=43", "data_length=0", fmt.Sprintf("actual_owner=%q", (solana.Address{}).String())}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &QuoteSnapshot{pool: pool, programID: program, accounts: solana.FrozenAccountState{Slot: 40, Accounts: tc.entries}}
			view, err := s.PoolSnapshot()
			if err == nil || view != nil {
				t.Fatalf("invalid snapshot accepted: %v %v", view, err)
			}
			for _, want := range append(tc.want, fmt.Sprintf("expected_owner=%q", program.String())) {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q missing %q", err, want)
				}
			}
		})
	}
}
