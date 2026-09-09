package solana

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
)

type accountStateReceiver struct {
	testAccountChanges
	update  *AccountUpdate
	failure error
}

// RecvState returns an injected account update.
//
// Version:
//   - 2026-09-09: Added.
func (r *accountStateReceiver) RecvState(context.Context) (*AccountUpdate, error) {
	return r.update, r.failure
}

func TestAccountStateSubscriptionPreservesPayloadAndOwnership(t *testing.T) {
	wire := &ws.AccountResult{}
	wire.Context.Slot = 42
	wire.Value.Owner = sdk.PublicKey{2}
	wire.Value.Lamports = 100
	wire.Value.Data = rpc.DataBytesOrJSONFromBytes([]byte{1, 2, 3})
	update, err := decodeAccountUpdate(Address{1}, wire)
	if err != nil {
		t.Fatal(err)
	}
	receiver := &accountStateReceiver{update: update}
	provider := &testAccountProvider{receiver: receiver}
	client := &WSClient{accounts: provider, commitment: CommitmentConfirmed}
	sub, err := client.SubscribeAccountChanges(Address{1})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	state, err := sub.RecvState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Slot != 42 || state.Account.Address != (Address{1}) || state.Account.Owner != (Address{2}) || state.Account.Lamports != 100 || len(state.Account.Data) != 3 {
		t.Fatalf("state=%+v account=%+v", state, state.Account)
	}
	wire.Value.Data.GetBinary()[0] = 8
	update.Account.Data[1] = 9
	if state.Account.Data[0] != 1 || state.Account.Data[1] != 2 {
		t.Fatal("retained bytes changed with transport buffers")
	}
	sentinel := errors.New("stream lost")
	receiver.failure = sentinel
	if _, err := sub.RecvState(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("error chain lost: %v", err)
	}
}

func TestAccountStateSubscriptionRejectsMissingPayload(t *testing.T) {
	for _, update := range []*AccountUpdate{nil, {}, {Slot: 1}, {Slot: 1, Account: &Account{}}, {Account: &Account{Address: Address{1}}}} {
		sub := &AccountChangeSubscription{receiver: &accountStateReceiver{update: update}}
		if _, err := sub.RecvState(context.Background()); err == nil {
			t.Fatalf("accepted invalid update: %+v", update)
		}
	}
	sub := &AccountChangeSubscription{receiver: &testAccountChanges{slot: 42}}
	if _, err := sub.RecvState(context.Background()); err == nil {
		t.Fatal("slot-only receiver reported state")
	}
}
