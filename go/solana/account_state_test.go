package solana

import (
	"testing"
	"time"
)

// TestAccountStateFreezesMixedPositionsAndOwnedBytes verifies retained snapshot behavior.
//
// Version:
//   - 2026-09-09: Added.
func TestAccountStateFreezesMixedPositionsAndOwnedBytes(t *testing.T) {
	var state AccountState
	var pool, tick Address
	pool[0], tick[0] = 1, 2
	now := time.Now()
	state.Seed([]*Account{{Address: pool, Data: []byte{1}}, {Address: tick, Data: []byte{2}}}, 10, now)
	update := &AccountUpdate{Slot: 11, Account: &Account{Address: pool, Data: []byte{3}}}
	if err := state.Apply(update, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	update.Account.Data[0] = 99
	frozen := state.Freeze()
	if frozen.Slot != 10 || frozen.Accounts[pool].Slot != 11 || frozen.Accounts[tick].Slot != 10 || frozen.Accounts[pool].Account.Data[0] != 3 {
		t.Fatalf("lost provenance or owned bytes: %+v", frozen)
	}
	if !frozen.ObservedAt.Equal(now) {
		t.Fatal("advanced old input time")
	}
	if err := state.Apply(&AccountUpdate{Slot: 12, Account: &Account{Address: tick, Data: []byte{4}}}, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if frozen.Accounts[tick].Account.Data[0] != 2 {
		t.Fatal("later notification mutated calculation")
	}
	frozen.Accounts[pool].Account.Data[0] = 98
	if state.Freeze().Accounts[pool].Account.Data[0] != 3 {
		t.Fatal("calculation mutated retained state")
	}
	state.Seed([]*Account{{Address: pool, Data: []byte{5}}}, 10, now)
	if err := state.Apply(&AccountUpdate{Slot: 9, Account: &Account{Address: pool, Data: []byte{6}}}, now); err != nil {
		t.Fatal(err)
	}
	if state.Freeze().Accounts[pool].Account.Data[0] != 3 {
		t.Fatal("older bootstrap or notification rewound state")
	}
}

// TestReceiptIgnoresDuplicates verifies that replay cannot refresh an unchanged input.
//
// Version:
//   - 2026-09-10: Added.
func TestReceiptIgnoresDuplicates(t *testing.T) {
	var state AccountState
	var address Address
	address[0] = 1
	received := time.Unix(1700000000, 0)
	update := &AccountUpdate{Slot: 10, Account: &Account{Address: address, Data: []byte{1}}}
	if err := state.Apply(update, received); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(update, received.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	frozen := state.Freeze()
	if !frozen.ReceivedAt.Equal(received) {
		t.Fatal("duplicate refreshed receipt")
	}
	update.Slot = 11
	if err := state.Apply(update, received.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !state.Freeze().ReceivedAt.Equal(received.Add(time.Second)) || !frozen.ReceivedAt.Equal(received) {
		t.Fatal("receipt or frozen provenance lost")
	}
}
