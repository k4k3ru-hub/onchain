package sui

import (
	"context"
	"testing"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"github.com/mr-tron/base58"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeLiveTransactionProvider struct {
	receiver liveTransactionReceiver
	address  Address
}

func (f *fakeLiveTransactionProvider) subscribeEvents(context.Context, LiveEventFilter) (liveEventReceiver, error) {
	return nil, nil
}

func (f *fakeLiveTransactionProvider) subscribeTransactions(_ context.Context, address Address) (liveTransactionReceiver, error) {
	f.address = address
	return f.receiver, nil
}

type fakeLiveTransactionReceiver struct {
	notification *TransactionNotification
	closed       int
}

func (f *fakeLiveTransactionReceiver) Recv() (*TransactionNotification, error) {
	return f.notification, nil
}

func (f *fakeLiveTransactionReceiver) Close() { f.closed++ }

func TestSubscribeTransactions(t *testing.T) {
	address, err := ParseAddress("0x2")
	if err != nil {
		t.Fatalf("ParseAddress() returned an unexpected error: %v", err)
	}
	receiver := &fakeLiveTransactionReceiver{notification: &TransactionNotification{Watermark: EventWatermark{Cursor: []byte("cursor")}}}
	provider := &fakeLiveTransactionProvider{receiver: receiver}
	client := composeGRPCClient(GRPCConfig{URL: "https://example.com"}, provider, nil)
	subscription, err := client.SubscribeTransactions(nil, address)
	if err != nil {
		t.Fatalf("SubscribeTransactions() returned an unexpected error: %v", err)
	}
	notification, err := subscription.Recv(nil)
	if err != nil {
		t.Fatalf("TransactionSubscription.Recv() returned an unexpected error: %v", err)
	}
	if string(notification.Watermark.Cursor) != "cursor" || provider.address != address {
		t.Fatalf("TransactionSubscription.Recv() = %+v address=%s", notification, provider.address)
	}
	subscription.Close()
	subscription.Close()
	if receiver.closed != 1 {
		t.Fatalf("TransactionSubscription.Close() count = %d, want 1", receiver.closed)
	}
}

func TestDecodeGRPCTransaction(t *testing.T) {
	digest := base58.Encode(make([]byte, digestByteLength))
	checkpoint := uint64(123)
	success := true
	from := "0x2"
	to := "0x3"
	coinType := "0x2::sui::SUI"
	negativeAmount := "-100"
	positiveAmount := "100"
	timestamp := timestamppb.New(time.Unix(1_700_000_000, 0))

	effects, err := decodeGRPCTransaction(&rpcv2.ExecutedTransaction{
		Digest:     &digest,
		Checkpoint: &checkpoint,
		Timestamp:  timestamp,
		Effects:    &rpcv2.TransactionEffects{Status: &rpcv2.ExecutionStatus{Success: &success}},
		BalanceChanges: []*rpcv2.BalanceChange{
			{Address: &from, CoinType: &coinType, Amount: &negativeAmount},
			{Address: &to, CoinType: &coinType, Amount: &positiveAmount},
		},
	})
	if err != nil {
		t.Fatalf("decodeGRPCTransaction() returned an unexpected error: %v", err)
	}
	if !effects.Successful || effects.Checkpoint == nil || *effects.Checkpoint != 123 || effects.Timestamp == nil {
		t.Fatalf("decodeGRPCTransaction() = %+v", effects)
	}
	if len(effects.BalanceChanges) != 2 || effects.BalanceChanges[0].Amount.String() != "-100" || effects.BalanceChanges[1].Amount.String() != "100" {
		t.Fatalf("decodeGRPCTransaction() balance changes = %+v", effects.BalanceChanges)
	}
}
