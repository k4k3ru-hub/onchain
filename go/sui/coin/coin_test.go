package coin

import (
	"context"
	"math/big"
	"testing"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type fakeProvider struct {
	metadata *onchainSui.CoinMetadata
	effects  *onchainSui.TransactionEffects
	page     onchainSui.TransactionDigestPage
}

func (f *fakeProvider) CoinMetadata(context.Context, string) (*onchainSui.CoinMetadata, error) {
	return f.metadata, nil
}
func (f *fakeProvider) TransactionEffects(context.Context, onchainSui.TransactionDigest) (*onchainSui.TransactionEffects, error) {
	return f.effects, nil
}
func (f *fakeProvider) TransactionDigests(context.Context, onchainSui.TransactionQuery) (onchainSui.TransactionDigestPage, error) {
	return f.page, nil
}

type fakeTransactionSubscription struct {
	notification *onchainSui.TransactionNotification
	closed       bool
}

func (f *fakeTransactionSubscription) Recv(context.Context) (*onchainSui.TransactionNotification, error) {
	return f.notification, nil
}
func (f *fakeTransactionSubscription) Close() { f.closed = true }

func TestClientMetadataAndTransfers(t *testing.T) {
	coinType, err := onchainSui.NormalizeMoveType("0x2::sui::SUI")
	if err != nil {
		t.Fatalf("NormalizeMoveType() returned an unexpected error: %v", err)
	}
	from, _ := onchainSui.ParseAddress("0x123")
	to, _ := onchainSui.ParseAddress("0x456")
	var digest onchainSui.TransactionDigest
	digest[0] = 1
	effects := &onchainSui.TransactionEffects{Digest: digest, Successful: true, BalanceChanges: []onchainSui.BalanceChange{
		{Address: from, CoinType: coinType, Amount: big.NewInt(-100)},
		{Address: to, CoinType: coinType, Amount: big.NewInt(100)},
	}}
	provider := &fakeProvider{
		metadata: &onchainSui.CoinMetadata{CoinType: coinType, Symbol: "SUI", Decimals: 9},
		effects:  effects,
		page:     onchainSui.TransactionDigestPage{Digests: []onchainSui.TransactionDigest{digest}, HasNextPage: true, NextCursor: "next"},
	}
	client, err := NewClient(provider, []string{"0x2::sui::SUI"})
	if err != nil {
		t.Fatalf("NewClient() returned an unexpected error: %v", err)
	}
	metadata, err := client.Metadata(nil, "0x2::sui::SUI")
	if err != nil || metadata.Symbol != "SUI" {
		t.Fatalf("Metadata() = %+v error=%v", metadata, err)
	}
	transfers, err := client.TransfersByTransaction(nil, digest)
	if err != nil {
		t.Fatalf("TransfersByTransaction() returned an unexpected error: %v", err)
	}
	if len(transfers) != 1 || transfers[0].From != from || transfers[0].To != to || transfers[0].Amount.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("TransfersByTransaction() = %+v, want one matching transfer", transfers)
	}
	page, err := client.Transfers(nil, TransferQuery{Address: from, CoinType: "0x2::sui::SUI"})
	if err != nil {
		t.Fatalf("Transfers() returned an unexpected error: %v", err)
	}
	if len(page.Transfers) != 1 || !page.HasNextPage || page.NextCursor != "next" {
		t.Fatalf("Transfers() = %+v, want paginated transfer", page)
	}
}

func TestTransferSubscription(t *testing.T) {
	coinType, _ := onchainSui.NormalizeMoveType("0x2::sui::SUI")
	from, _ := onchainSui.ParseAddress("0x123")
	to, _ := onchainSui.ParseAddress("0x456")
	var digest onchainSui.TransactionDigest
	digest[0] = 2
	effects := &onchainSui.TransactionEffects{Digest: digest, Successful: true, BalanceChanges: []onchainSui.BalanceChange{
		{Address: from, CoinType: coinType, Amount: big.NewInt(-25)}, {Address: to, CoinType: coinType, Amount: big.NewInt(25)},
	}}
	client, err := NewClient(&fakeProvider{}, []string{"0x2::sui::SUI"})
	if err != nil {
		t.Fatalf("NewClient() returned an unexpected error: %v", err)
	}
	transactions := &fakeTransactionSubscription{notification: &onchainSui.TransactionNotification{Effects: effects}}
	subscriber, err := NewStandardTransferSubscriber(client, func(context.Context, onchainSui.Address) (TransactionSubscription, error) {
		return transactions, nil
	})
	if err != nil {
		t.Fatalf("NewStandardTransferSubscriber() returned an unexpected error: %v", err)
	}
	client.WithTransferSubscriber(subscriber)
	subscription, err := client.SubscribeTransfers(nil, TransferSubscriptionFilter{Address: to, CoinType: "0x2::sui::SUI"})
	if err != nil {
		t.Fatalf("SubscribeTransfers() returned an unexpected error: %v", err)
	}
	transfer, err := subscription.Recv(nil)
	if err != nil {
		t.Fatalf("TransferSubscription.Recv() returned an unexpected error: %v", err)
	}
	if transfer.To != to || transfer.Amount.Cmp(big.NewInt(25)) != 0 {
		t.Fatalf("TransferSubscription.Recv() = %+v, want matching transfer", transfer)
	}
	subscription.Close()
	if !transactions.closed {
		t.Fatal("TransferSubscription.Close() did not close transaction subscription")
	}
}
