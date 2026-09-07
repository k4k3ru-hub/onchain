package solana

import (
	"context"
	"errors"
	"testing"
)

type testAccountChanges struct {
	slot   Slot
	err    error
	closes int
}

// Recv returns a fake notification.
//
// Version:
//   - 2026-09-07: Added.
func (r *testAccountChanges) Recv(ctx context.Context) (Slot, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.slot, r.err
}

// Unsubscribe counts subscription releases.
//
// Version:
//   - 2026-09-07: Added.
func (r *testAccountChanges) Unsubscribe() { r.closes++ }

type testAccountProvider struct {
	receiver   accountChangeReceiver
	err        error
	address    Address
	commitment Commitment
}

func (p *testAccountProvider) subscribeAccountChanges(a Address, c Commitment) (accountChangeReceiver, error) {
	p.address, p.commitment = a, c
	return p.receiver, p.err
}
func TestAccountChangesTransport(t *testing.T) {
	r := &testAccountChanges{slot: 42}
	p := &testAccountProvider{receiver: r}
	c := &WSClient{accounts: p, commitment: CommitmentConfirmed}
	address := Address{1}
	sub, err := c.SubscribeAccountChanges(address)
	if err != nil {
		t.Fatal(err)
	}
	if p.address != address || p.commitment != CommitmentConfirmed {
		t.Fatal("subscription parameters lost")
	}
	if slot, err := sub.Recv(context.Background()); err != nil || slot != 42 {
		t.Fatalf("slot=%d err=%v", slot, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sub.Recv(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	r.slot = 0
	if _, err := sub.Recv(context.Background()); err == nil {
		t.Fatal("invalid slot accepted")
	}
	sub.Close()
	sub.Close()
	if r.closes != 1 {
		t.Fatal("subscription released more than once")
	}
	failure := errors.New("transport failure")
	p.err = failure
	if _, err := c.SubscribeAccountChanges(address); !errors.Is(err, failure) {
		t.Fatalf("subscription error lost: %v", err)
	}
	p.err = nil
	p.receiver = nil
	if _, err := c.SubscribeAccountChanges(address); err == nil {
		t.Fatal("nil receiver accepted")
	}
}
