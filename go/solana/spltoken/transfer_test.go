package spltoken

import (
	"context"
	"testing"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type stubTransactionSource struct {
	signature   onchainSolana.Signature
	transaction *onchainSolana.Transaction
}

func (s stubTransactionSource) SignaturesForAddress(context.Context, onchainSolana.Address, int) ([]onchainSolana.Signature, error) {
	return []onchainSolana.Signature{s.signature}, nil
}
func (s stubTransactionSource) Transaction(context.Context, onchainSolana.Signature) (*onchainSolana.Transaction, error) {
	return s.transaction, nil
}

func TestStandardTransferProvider(t *testing.T) {
	var signature onchainSolana.Signature
	signature[0] = 1
	var address, from, to, mint onchainSolana.Address
	address[0] = 1
	from[0] = 2
	to[0] = 3
	mint[0] = 4
	transaction := &onchainSolana.Transaction{Signature: signature, Slot: 10,
		PreTokenBalances:  []onchainSolana.TokenBalance{{AccountIndex: 1, Owner: &from, Mint: mint, Amount: "100", Decimals: 6}, {AccountIndex: 2, Owner: &to, Mint: mint, Amount: "0", Decimals: 6}},
		PostTokenBalances: []onchainSolana.TokenBalance{{AccountIndex: 1, Owner: &from, Mint: mint, Amount: "60", Decimals: 6}, {AccountIndex: 2, Owner: &to, Mint: mint, Amount: "40", Decimals: 6}},
	}
	provider, err := NewStandardTransferProvider(stubTransactionSource{signature: signature, transaction: transaction})
	if err != nil {
		t.Fatal(err)
	}
	events, err := provider.TransferEvents(nil, TransferFilter{Address: address, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Amount != "40" || events[0].From != from || events[0].To != to {
		t.Fatalf("TransferEvents() = %+v", events)
	}
}
