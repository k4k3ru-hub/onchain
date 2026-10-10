package sui

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type poolEventsTransport struct {
	rpcv2.SubscriptionServiceClient
	request  *rpcv2.SubscribeTransactionsRequest
	response *rpcv2.SubscribeTransactionsResponse
	ctx      context.Context
}

// SubscribeTransactions injects a deterministic stream at the client boundary.
//
// Version:
//   - 2026-10-08: Added.
func (f *poolEventsTransport) SubscribeTransactions(ctx context.Context, request *rpcv2.SubscribeTransactionsRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[rpcv2.SubscribeTransactionsResponse], error) {
	f.ctx, f.request = ctx, request
	return &poolEventsWire{ctx: ctx, response: f.response}, nil
}

type poolEventsWire struct {
	grpc.ClientStream
	ctx      context.Context
	response *rpcv2.SubscribeTransactionsResponse
}

// Recv preserves the supplied payload and subscription cancellation.
//
// Version:
//   - 2026-10-08: Added.
func (w *poolEventsWire) Recv() (*rpcv2.SubscribeTransactionsResponse, error) {
	if err := w.ctx.Err(); err != nil {
		return nil, err
	}
	return w.response, nil
}

// TestPoolEventsComposition verifies BCS, ledger ordering, timestamps and closing.
//
// Version:
//   - 2026-10-08: Added.
func TestPoolEventsComposition(t *testing.T) {
	id, err := ParseAddress("0x9")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 8, 12, 0, 0, 123000000, time.UTC)
	event := &rpcv2.Event{PackageId: proto.String("0x1"), Module: proto.String("pool"), Sender: proto.String("0x2"),
		EventType: proto.String("0x1::pool::SwapEvent"), Contents: &rpcv2.Bcs{Value: []byte{1, 2, 3}}}
	tx := &rpcv2.ExecutedTransaction{Digest: proto.String(strings.Repeat("1", 32)), Checkpoint: proto.Uint64(42),
		TransactionIndex: proto.Uint64(0), Timestamp: timestamppb.New(at),
		Effects: &rpcv2.TransactionEffects{Status: &rpcv2.ExecutionStatus{Success: proto.Bool(true)}},
		Events:  &rpcv2.TransactionEvents{Events: []*rpcv2.Event{event}},
		Objects: &rpcv2.ObjectSet{Objects: []*rpcv2.Object{nil}}}
	transport := &poolEventsTransport{response: &rpcv2.SubscribeTransactionsResponse{Transaction: tx,
		Watermark: &rpcv2.Watermark{Cursor: []byte{1}, Checkpoint: proto.Uint64(42)}}}
	client := composeGRPCClient(GRPCConfig{URL: "https://example.com"}, &grpcAdapter{client: transport}, nil)
	sub, err := client.SubscribePoolEvents(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	request := transport.request
	if request.Filter.Terms[0].Literals[0].GetAffectedObject().GetObjectId() != id.String() {
		t.Fatal("wrong affected pool")
	}
	for _, path := range []string{"digest", "effects.status", "checkpoint", "transaction_index", "timestamp",
		"events.events.package_id", "events.events.module", "events.events.sender", "events.events.event_type", "events.events.contents"} {
		if !slices.Contains(request.ReadMask.Paths, path) {
			t.Fatalf("missing required field %s", path)
		}
	}
	for _, path := range request.ReadMask.Paths {
		if strings.HasPrefix(path, "objects") || path == "events" || strings.HasSuffix(path, ".json") || path == "balance_changes" {
			t.Fatalf("unneeded field requested: %s", path)
		}
	}
	got, err := sub.Recv(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.EventsError != nil || got.ObjectChangesError != nil || len(got.Events) != 1 || len(got.ObjectChanges) != 0 {
		t.Fatalf("bad notification: %+v", got)
	}
	if got.Effects.Timestamp == nil || !got.Effects.Timestamp.Equal(at) || got.Events[0].Checkpoint != 42 || got.Events[0].TransactionIndex != 0 || got.Events[0].EventIndex != 0 || got.Events[0].Transaction != got.Effects.Digest {
		t.Fatal("lost transaction time or ledger position")
	}
	if !reflect.DeepEqual(got.Events[0].BCS, event.Contents.Value) || len(got.Events[0].JSON) != 0 {
		t.Fatal("changed BCS event")
	}
	tx.Effects.Status.Success = proto.Bool(false)
	got, err = sub.Recv(t.Context())
	if err != nil || got.Effects.Successful || len(got.Events) != 0 {
		t.Fatal("unsuccessful transaction produced events")
	}
	transport.response.Transaction = nil
	got, err = sub.Recv(t.Context())
	if err != nil || got.Effects != nil || len(got.Events) != 0 || len(got.Watermark.Cursor) == 0 {
		t.Fatal("progress notification lost")
	}
	sub.Close()
	if !errors.Is(transport.ctx.Err(), context.Canceled) {
		t.Fatal("Close did not cancel the upstream stream")
	}
}

// TestPoolEventsTransportErrors preserves transport errors and rejects nil streams.
//
// Version:
//   - 2026-10-08: Added.
func TestPoolEventsTransportErrors(t *testing.T) {
	id, err := ParseAddress("0x9")
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("unavailable")
	transport := &objectStreamClient{err: sentinel}
	client := composeGRPCClient(GRPCConfig{URL: "https://example.com"}, &grpcAdapter{client: transport}, nil)
	if _, err := client.SubscribePoolEvents(t.Context(), id); !errors.Is(err, sentinel) {
		t.Fatalf("lost transport error: %v", err)
	}
	transport.err = nil
	if _, err := client.SubscribePoolEvents(t.Context(), id); err == nil {
		t.Fatal("nil upstream accepted")
	}
	if _, err := client.SubscribePoolEvents(nil, id); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := client.SubscribePoolEvents(t.Context(), Address{}); err == nil {
		t.Fatal("empty pool accepted")
	}
}
