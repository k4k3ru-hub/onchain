package solana

import (
	"testing"
	"time"
)

// TestSeedWindowPrunesAndPreservesLiveInputs verifies bounded retention and same/newer-slot precedence.
//
// Version:
//   - 2026-09-11: Added.
func TestSeedWindowPrunesAndPreservesLiveInputs(t *testing.T) {
	var first, second, third Address
	first[31] = 1
	second[31] = 2
	third[31] = 3
	before := time.Now().Add(-time.Minute)
	state := &AccountState{}
	state.Seed([]*Account{{Address: first, Data: []byte{1}}, {Address: second, Data: []byte{2}}}, 10, before)
	frozen := state.Freeze()
	if err := state.Apply(&AccountUpdate{Slot: 12, Account: &Account{Address: second, Data: []byte{12}}}, before); err != nil {
		t.Fatal(err)
	}
	state.SeedWindow([]*Account{{Address: second, Data: []byte{0}}, {Address: third, Data: []byte{3}}}, 12, time.Now())
	result := state.Freeze()
	if len(result.Accounts) != 2 || result.Accounts[first].Account != nil || result.Accounts[second].Account.Data[0] != 12 || !result.Accounts[second].ObservedAt.Equal(before) {
		t.Fatal("window did not preserve bounded live state")
	}
	if frozen.Accounts[first].Account.Data[0] != 1 || frozen.Accounts[second].Account.Data[0] != 2 {
		t.Fatal("previous frozen inputs mutated")
	}
}
