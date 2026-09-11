package sui

import (
	"context"
	"errors"
	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"strings"
	"testing"
)

// TestPoolTransactionComposition verifies events and state share a single object-filtered request.
//
// Version:
//   - 2026-09-11: Added.
func TestPoolTransactionComposition(t *testing.T) {
	sentinel := errors.New("unavailable")
	transport := &objectStreamClient{err: sentinel}
	client := composeGRPCClient(GRPCConfig{URL: "https://example.com"}, &grpcAdapter{client: transport}, nil)
	id, err := ParseAddress("0x9")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubscribePoolTransactions(context.Background(), id); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	mask := strings.Join(transport.request.ReadMask.Paths, ",")
	for _, field := range []string{"events", "objects", "effects.changed_objects", "transaction_index"} {
		if !strings.Contains(mask, field) {
			t.Fatal(mask)
		}
	}
	if transport.request.Filter.Terms[0].Literals[0].GetAffectedObject().GetObjectId() != id.String() {
		t.Fatal("wrong pool filter")
	}
}

type poolTransactionWire struct {
	response *rpcv2.SubscribeTransactionsResponse
}

func (f *poolTransactionWire) Recv() (*rpcv2.SubscribeTransactionsResponse, error) {
	return f.response, nil
}

// TestPoolTransactionEventsSurviveObjectDecodeFailure verifies independent decoding and inherited order.
//
// Version:
//   - 2026-09-11: Added.
func TestPoolTransactionEventsSurviveObjectDecodeFailure(t *testing.T) {
	jsonValue, err := structpb.NewValue(map[string]any{"pool": "0x9"})
	if err != nil {
		t.Fatal(err)
	}
	ev := &rpcv2.Event{PackageId: proto.String("0x1"), Sender: proto.String("0x2"), Module: proto.String("pool"), EventType: proto.String("0x1::pool::SwapEvent"), Json: jsonValue}
	tx := &rpcv2.ExecutedTransaction{Digest: proto.String(strings.Repeat("1", 32)), Checkpoint: proto.Uint64(42), TransactionIndex: proto.Uint64(7), Effects: &rpcv2.TransactionEffects{Status: &rpcv2.ExecutionStatus{Success: proto.Bool(true)}}, Events: &rpcv2.TransactionEvents{Events: []*rpcv2.Event{nil, ev}}, Objects: &rpcv2.ObjectSet{Objects: []*rpcv2.Object{nil}}}
	r := &grpcTransactionReceiver{includeObjects: true, includeEvents: true, stream: &poolTransactionWire{response: &rpcv2.SubscribeTransactionsResponse{Transaction: tx, Watermark: &rpcv2.Watermark{Cursor: []byte{1}}}}}
	n, err := r.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if n.ObjectChangesError == nil || n.EventsError == nil || len(n.Events) != 1 {
		t.Fatalf("independent errors/events: %+v", n)
	}
	got := n.Events[0]
	if got.Checkpoint != 42 || got.TransactionIndex != 7 || got.EventIndex != 1 {
		t.Fatalf("order: %+v", got)
	}
	if ev.Checkpoint != nil || ev.EventIndex != nil {
		t.Fatal("mutated transport event")
	}
}

// TestPoolEventsDoNotInventTransactionOrder verifies absent order stays unavailable.
//
// Version:
//   - 2026-09-11: Added.
func TestPoolEventsDoNotInventTransactionOrder(t *testing.T) {
	tx := &rpcv2.ExecutedTransaction{Digest: proto.String(strings.Repeat("1", 32)), Checkpoint: proto.Uint64(42), Events: &rpcv2.TransactionEvents{Events: []*rpcv2.Event{{PackageId: proto.String("0x1"), Sender: proto.String("0x2"), Module: proto.String("pool"), EventType: proto.String("0x1::pool::SwapEvent")}}}}
	events, err := decodeTransactionEvents(tx)
	if err == nil || len(events) != 0 {
		t.Fatal("fabricated transaction index")
	}
	tx.TransactionIndex = proto.Uint64(0)
	events, err = decodeTransactionEvents(tx)
	if err != nil || len(events) != 1 || events[0].TransactionIndex != 0 {
		t.Fatalf("valid zero index: %v %+v", err, events)
	}
}
