package sui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/structpb"
)

type fakeLiveEventProvider struct {
	receiver liveEventReceiver
	filter   LiveEventFilter
}

func (f *fakeLiveEventProvider) subscribeEvents(_ context.Context, filter LiveEventFilter) (liveEventReceiver, error) {
	f.filter = filter
	return f.receiver, nil
}

type fakeLiveEventReceiver struct {
	notification *EventNotification
	closed       int
}

func (f *fakeLiveEventReceiver) Recv() (*EventNotification, error) { return f.notification, nil }
func (f *fakeLiveEventReceiver) Close()                            { f.closed++ }

type fakeGRPCCloser struct {
	closed int
	err    error
}

func (f *fakeGRPCCloser) Close() error {
	f.closed++
	return f.err
}

func TestGRPCConfigValidate(t *testing.T) {
	for _, value := range []string{"https://fullnode.mainnet.sui.io:443", "fullnode.testnet.sui.io:443", "http://localhost:9000"} {
		if err := (GRPCConfig{URL: value}).Validate(); err != nil {
			t.Fatalf("GRPCConfig.Validate(%q) returned an unexpected error: %v", value, err)
		}
	}
	if err := (GRPCConfig{}).Validate(); err == nil {
		t.Fatal("GRPCConfig.Validate() error = nil, want error")
	}
}

func TestSubscribeEvents(t *testing.T) {
	sender, err := ParseAddress("0x123")
	if err != nil {
		t.Fatalf("ParseAddress() returned an unexpected error: %v", err)
	}
	receiver := &fakeLiveEventReceiver{notification: &EventNotification{Watermark: EventWatermark{Cursor: []byte("cursor")}}}
	provider := &fakeLiveEventProvider{receiver: receiver}
	client := composeGRPCClient(GRPCConfig{URL: "https://example.com"}, provider, nil)
	subscription, err := client.SubscribeEvents(nil, LiveEventFilter{Sender: &sender, Module: "0x2::coin", Type: "0x2::coin::Transfer"})
	if err != nil {
		t.Fatalf("SubscribeEvents() returned an unexpected error: %v", err)
	}
	notification, err := subscription.Recv(nil)
	if err != nil {
		t.Fatalf("EventSubscription.Recv() returned an unexpected error: %v", err)
	}
	if string(notification.Watermark.Cursor) != "cursor" || provider.filter.Sender == nil || *provider.filter.Sender != sender {
		t.Fatalf("EventSubscription.Recv() = %+v filter=%+v", notification, provider.filter)
	}
	subscription.Close()
	subscription.Close()
	if receiver.closed != 1 {
		t.Fatalf("EventSubscription.Close() count = %d, want 1", receiver.closed)
	}
}

func TestGRPCClientClose(t *testing.T) {
	closeError := errors.New("test close error")
	closer := &fakeGRPCCloser{err: closeError}
	client := composeGRPCClient(GRPCConfig{}, nil, closer)
	if err := client.Close(); !errors.Is(err, closeError) {
		t.Fatalf("GRPCClient.Close() error = %v, want wrapped close error", err)
	}
	if err := client.Close(); !errors.Is(err, closeError) || closer.closed != 1 {
		t.Fatalf("GRPCClient.Close() second error/count = %v/%d", err, closer.closed)
	}
}

func TestMakeGRPCEventFilter(t *testing.T) {
	sender, err := ParseAddress("0x123")
	if err != nil {
		t.Fatalf("ParseAddress() returned an unexpected error: %v", err)
	}
	filter, err := makeGRPCEventFilter(LiveEventFilter{Sender: &sender, Module: "0x2::coin", Type: "0x2::coin::Transfer"})
	if err != nil {
		t.Fatalf("makeGRPCEventFilter() returned an unexpected error: %v", err)
	}
	if filter == nil || len(filter.Terms) != 1 || len(filter.Terms[0].Literals) != 3 {
		t.Fatalf("makeGRPCEventFilter() = %+v, want one term with three literals", filter)
	}
	if got := filter.Terms[0].Literals[0].GetSender().GetAddress(); got != sender.String() {
		t.Fatalf("makeGRPCEventFilter() sender = %q, want %q", got, sender)
	}
	if got := filter.Terms[0].Literals[1].GetEmitModule().GetModule(); !strings.HasPrefix(got, "0x0000000000000000000000000000000000000000000000000000000000000002::coin") {
		t.Fatalf("makeGRPCEventFilter() module = %q, want canonical package address", got)
	}
}

func TestDecodeGRPCEvent(t *testing.T) {
	sender, _ := ParseAddress("0x123")
	packageAddress, _ := ParseAddress("0x2")
	transaction := base58DigestWithFirstByte(13)
	checkpoint := uint64(100)
	transactionIndex := uint64(2)
	eventIndex := uint32(0)
	module := "coin"
	eventType := "0x2::coin::Transfer"
	senderValue := sender.String()
	packageValue := packageAddress.String()
	jsonValue, err := structpb.NewValue(map[string]any{"amount": "100"})
	if err != nil {
		t.Fatalf("structpb.NewValue() returned an unexpected error: %v", err)
	}
	event, err := decodeGRPCEvent(&rpcv2.Event{
		Checkpoint: &checkpoint, TransactionDigest: &transaction, TransactionIndex: &transactionIndex, EventIndex: &eventIndex,
		Sender: &senderValue, PackageId: &packageValue, Module: &module, EventType: &eventType,
		Contents: &rpcv2.Bcs{Value: []byte{1, 2, 3}}, Json: jsonValue,
	})
	if err != nil {
		t.Fatalf("decodeGRPCEvent() returned an unexpected error: %v", err)
	}
	if event.Checkpoint != 100 || event.EventIndex != 0 || event.Sender != sender || event.Transaction.String() != transaction || string(event.BCS) != string([]byte{1, 2, 3}) {
		t.Fatalf("decodeGRPCEvent() = %+v, want matching event", event)
	}
	var decoded map[string]any
	if err := json.Unmarshal(event.JSON, &decoded); err != nil || decoded["amount"] != "100" {
		t.Fatalf("decodeGRPCEvent() JSON = %s error=%v", event.JSON, err)
	}
}

func TestLiveEventFilterValidate(t *testing.T) {
	value := strings.Repeat("x", 513)
	if err := (LiveEventFilter{Module: value}).Validate(); err == nil || !strings.Contains(err.Error(), "module=too_long") {
		t.Fatalf("LiveEventFilter.Validate() error = %v, want module too long", err)
	}
}

func base58DigestWithFirstByte(value byte) string {
	var digest TransactionDigest
	digest[0] = value
	return digest.String()
}
