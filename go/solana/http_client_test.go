package solana

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type injectedHTTPTransport func(*http.Request) (*http.Response, error)

func (f injectedHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestRPCClientUsesInjectedHTTPClient(t *testing.T) {
	calls := 0
	transport := injectedHTTPTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","result":42}`))}, nil
	})
	client, err := NewRPCClient(context.Background(), RPCConfig{URL: "http://rpc.test", Commitment: CommitmentConfirmed, HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	slot, err := client.Slot(context.Background())
	if err != nil || slot != 42 || calls != 1 {
		t.Fatalf("slot=%d calls=%d err=%v", slot, calls, err)
	}
}
