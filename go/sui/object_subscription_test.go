package sui

import (
	"context"
	"errors"
	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
	"testing"
)

type objectStreamClient struct {
	rpcv2.SubscriptionServiceClient
	request *rpcv2.SubscribeTransactionsRequest
	err     error
}

// SubscribeTransactions captures the wire request for a filtered transaction stream.
//
// Version:
//   - 2026-09-09: Added.
func (f *objectStreamClient) SubscribeTransactions(_ context.Context, r *rpcv2.SubscribeTransactionsRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[rpcv2.SubscribeTransactionsResponse], error) {
	f.request = r
	return nil, f.err
}
func TestObjectSubscriptionUsesObjectFilterAndPreservesTransportError(t *testing.T) {
	sentinel := errors.New("stream unavailable")
	transport := &objectStreamClient{err: sentinel}
	adapter := &grpcAdapter{client: transport}
	client := composeGRPCClient(GRPCConfig{URL: "https://example.com"}, adapter, nil)
	object, err := ParseAddress("0x2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.SubscribeObjectTransactions(context.Background(), object); !errors.Is(err, sentinel) {
		t.Fatalf("transport error lost: %v", err)
	}
	request := transport.request
	if request == nil {
		t.Fatal("stream not composed")
	}
	literal := request.Filter.Terms[0].Literals[0]
	if literal.GetAffectedObject() == nil || literal.GetAffectedObject().GetObjectId() != object.String() || literal.GetAffectedAddress() != nil {
		t.Fatal("pool object filtered as wallet address")
	}
	for _, field := range request.ReadMask.Paths {
		if field == "balance_changes" {
			t.Fatal("quote state depends on trade balance decoding")
		}
	}
}
