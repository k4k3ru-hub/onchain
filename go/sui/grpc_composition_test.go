package sui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
)

type compositionConnection struct {
	invoked, streamed []string
	err, closeErr     error
	closed            int
}

// Invoke records unary routing through the constructor's connection graph.
//
// Version:
//   - 2026-10-12: Added.
func (c *compositionConnection) Invoke(_ context.Context, method string, _, _ any, _ ...grpc.CallOption) error {
	c.invoked = append(c.invoked, method)
	return c.err
}

// NewStream records streaming routing without opening a network connection.
//
// Version:
//   - 2026-10-12: Added.
func (c *compositionConnection) NewStream(_ context.Context, _ *grpc.StreamDesc, method string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	c.streamed = append(c.streamed, method)
	return nil, c.err
}

// Close records ownership and simulates a close failure.
//
// Version:
//   - 2026-10-12: Added.
func (c *compositionConnection) Close() error { c.closed++; return c.closeErr }

// TestGRPCClientComposesIndependentLedger verifies scoped reads and streams use separate connections.
//
// Version:
//   - 2026-10-12: Added.
func TestGRPCClientComposesIndependentLedger(t *testing.T) {
	streamErr, readErr := errors.New("stream offline"), errors.New("read offline")
	stream, ledger := &compositionConnection{err: streamErr}, &compositionConnection{err: readErr}
	calls := 0
	client, err := newGRPCClient(GRPCConfig{URL: "https://example.com"}, func(target string, options ...grpc.DialOption) (grpcConnection, error) {
		calls++
		if target != "dns:///example.com:443" || len(options) != 1 {
			t.Fatal("connection configuration lost")
		}
		if calls == 1 {
			return stream, nil
		}
		return ledger, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("composition failed: %v", err)
	}
	if _, err := client.SubscribeObjectStateScoped(t.Context(), scopedAddress(t, "0x9"), ObjectStateScope{}); !errors.Is(err, streamErr) {
		t.Fatalf("wrong stream transport: %v", err)
	}
	adapter := client.transactionProvider.(*grpcAdapter)
	if _, err := adapter.ledgerClient.GetObject(t.Context(), &rpcv2.GetObjectRequest{}); !errors.Is(err, readErr) {
		t.Fatalf("wrong ledger transport: %v", err)
	}
	if len(stream.streamed) != 1 || len(stream.invoked) != 0 || len(ledger.invoked) != 1 || len(ledger.streamed) != 0 || !strings.HasSuffix(ledger.invoked[0], "/GetObject") {
		t.Fatal("ledger and subscription transport mixed")
	}
	closeA, closeB := errors.New("stream close"), errors.New("ledger close")
	stream.closeErr, ledger.closeErr = closeA, closeB
	for i := 0; i < 2; i++ {
		err := client.Close()
		if !errors.Is(err, closeA) || !errors.Is(err, closeB) {
			t.Fatalf("lost close error: %v", err)
		}
	}
	if stream.closed != 1 || ledger.closed != 1 {
		t.Fatal("connections leaked or closed twice")
	}
}

// TestGRPCClientCompositionFailure verifies cleanup when the second connection cannot be created.
//
// Version:
//   - 2026-10-12: Added.
func TestGRPCClientCompositionFailure(t *testing.T) {
	connectErr, closeErr := errors.New("connect failed"), errors.New("close failed")
	stream := &compositionConnection{closeErr: closeErr}
	calls := 0
	client, err := newGRPCClient(GRPCConfig{URL: "https://example.com"}, func(string, ...grpc.DialOption) (grpcConnection, error) {
		calls++
		if calls == 1 {
			return stream, nil
		}
		return nil, connectErr
	})
	if client != nil || !errors.Is(err, connectErr) || !errors.Is(err, closeErr) || stream.closed != 1 {
		t.Fatalf("partial composition leaked resources: %v", err)
	}
}
