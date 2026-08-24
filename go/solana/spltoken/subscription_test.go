package spltoken

import (
	"context"
	"testing"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type stubLogSubscription struct {
	log    *onchainSolana.Log
	closed bool
}

func (s *stubLogSubscription) Recv(context.Context) (*onchainSolana.Log, error) { return s.log, nil }
func (s *stubLogSubscription) Close()                                           { s.closed = true }

func TestTransferSubscription(t *testing.T) {
	var signature onchainSolana.Signature
	signature[0] = 1
	var from, to, mint onchainSolana.Address
	from[0] = 1
	to[0] = 2
	mint[0] = 3
	transaction := &onchainSolana.Transaction{Signature: signature, Slot: 10,
		PreTokenBalances:  []onchainSolana.TokenBalance{{AccountIndex: 1, Owner: &from, Mint: mint, Amount: "50"}, {AccountIndex: 2, Owner: &to, Mint: mint, Amount: "0"}},
		PostTokenBalances: []onchainSolana.TokenBalance{{AccountIndex: 1, Owner: &from, Mint: mint, Amount: "20"}, {AccountIndex: 2, Owner: &to, Mint: mint, Amount: "30"}},
	}
	source := stubTransactionSource{signature: signature, transaction: transaction}
	logs := &stubLogSubscription{log: &onchainSolana.Log{Signature: signature, Slot: 10}}
	subscriber, err := NewStandardTransferSubscriber(source, func(onchainSolana.Address) (LogSubscription, error) { return logs, nil })
	if err != nil {
		t.Fatalf("NewStandardTransferSubscriber() returned an unexpected error: %v", err)
	}
	subscription, err := subscriber.SubscribeTransfers(nil, TransferFilter{Address: from, Mint: &mint})
	if err != nil {
		t.Fatalf("SubscribeTransfers() returned an unexpected error: %v", err)
	}
	event, err := subscription.Recv(nil)
	if err != nil {
		t.Fatalf("Recv() returned an unexpected error: %v", err)
	}
	if event.Amount != "30" || event.From != from || event.To != to {
		t.Fatalf("Recv() = %+v, want 30-unit transfer", event)
	}
	subscription.Close()
	if !logs.closed {
		t.Fatal("Close() did not close log subscription")
	}
}
