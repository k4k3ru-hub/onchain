package pro

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip serves an injected HTTP response.
//
// Version:
//   - 2026-10-14: Added.
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestCompositionAndOfficialEnvironment verifies the named protocol behavior.
//
// Version:
//   - 2026-10-14: Added.
func TestCompositionAndOfficialEnvironment(t *testing.T) {
	for _, network := range []Network{Testnet, Mainnet} {
		calls := 0
		client, err := NewClient(Config{Network: network, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			want := "api.sui-staging.bluefin.io"
			if network == Mainnet {
				want = "api.sui-prod.bluefin.io"
			}
			if r.URL.Host != want || r.URL.Path != "/v1/exchange/info" || r.Header.Get("Authorization") != "" {
				t.Fatal("public request was routed incorrectly")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"contractsConfig":{"network":"` + string(network) + `"}}`)), Header: make(http.Header)}, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		if calls != 0 || client.Exchange() == nil || client.Auth() == nil || client.Trade() == nil || client.Account() == nil {
			t.Fatal("composition did not initialize all groups without network calls")
		}
		result, err := client.Exchange().Info(context.Background())
		if err != nil || result.Contracts.Network != network || calls != 1 {
			t.Fatal(result, err, calls)
		}
	}
	if _, err := NewClient(Config{Network: "invalid"}); err == nil {
		t.Fatal("invalid environment accepted")
	}
}

// TestRedirectAndUpstreamErrorPrivacy verifies the named protocol behavior.
//
// Version:
//   - 2026-10-14: Added.
func TestRedirectAndUpstreamErrorPrivacy(t *testing.T) {
	for _, status := range []int{302, 401, 429, 503} {
		calls := 0
		client, err := NewClient(Config{Network: Testnet, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://untrusted.invalid"}, "Retry-After": {"60"}}, Body: io.NopCloser(strings.NewReader("PRIVATE_UPSTREAM_BODY"))}, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Account().Get(context.Background(), "PRIVATE_BEARER_TOKEN")
		var upstream *HTTPError
		if !errors.As(err, &upstream) || upstream.StatusCode != status || calls != 1 {
			t.Fatal(err, calls)
		}
		if strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("credential or upstream body leaked")
		}
	}
	transportErr := errors.New("PRIVATE_TRANSPORT_URL")
	client, err := NewClient(Config{Network: Testnet, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, transportErr })})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Exchange().Info(context.Background())
	if !errors.Is(err, transportErr) || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("transport cause or privacy lost")
	}
}

type fakeConnection struct {
	messages [][]byte
	sent     []byte
	closed   bool
}

// Read returns a scripted stream frame.
//
// Version:
//   - 2026-10-14: Added.
func (c *fakeConnection) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(c.messages) == 0 {
		return nil, io.EOF
	}
	message := c.messages[0]
	c.messages = c.messages[1:]
	return message, nil
}

// Write captures a subscription frame.
//
// Version:
//   - 2026-10-14: Added.
func (c *fakeConnection) Write(_ context.Context, message []byte) error {
	c.sent = append([]byte(nil), message...)
	return nil
}

// Close records stream cleanup.
//
// Version:
//   - 2026-10-14: Added.
func (c *fakeConnection) Close() error { c.closed = true; return nil }

type fakeDialer struct {
	connection *fakeConnection
	endpoint   string
	header     http.Header
}

// Dial captures the official endpoint and authentication headers.
//
// Version:
//   - 2026-10-14: Added.
func (d *fakeDialer) Dial(_ context.Context, endpoint string, header http.Header) (Connection, error) {
	d.endpoint = endpoint
	d.header = header
	return d.connection, nil
}

// TestStreamAcknowledgmentAndBufferedEvent verifies the named protocol behavior.
//
// Version:
//   - 2026-10-14: Added.
func TestStreamAcknowledgmentAndBufferedEvent(t *testing.T) {
	connection := &fakeConnection{messages: [][]byte{[]byte(`{"event":"TickerUpdate","payload":{"symbol":"SUI-PERP"}}`), []byte(`{"success":true}`)}}
	dialer := &fakeDialer{connection: connection}
	client, err := NewStreamClient(Testnet, dialer)
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := client.Market(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dialer.endpoint != "wss://stream.api.sui-staging.bluefin.io/ws/market" || dialer.header.Get("Authorization") != "" {
		t.Fatal("market stream route or authentication incorrect")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(connection.sent, &payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload["dataStreams"]), "Partial_Depth_20") {
		t.Fatal("partial snapshots not subscribed")
	}
	event, err := subscription.Next(context.Background())
	if err != nil || event.Type != "TickerUpdate" {
		t.Fatal(event, err)
	}
	if err := subscription.Close(); err != nil || !connection.closed {
		t.Fatal("socket not closed", err)
	}
	connection = &fakeConnection{messages: [][]byte{[]byte(`{"success":false,"message":"PRIVATE_UPSTREAM_BODY"}`)}}
	dialer.connection = connection
	_, err = client.Account(context.Background(), "PRIVATE_BEARER_TOKEN")
	if err == nil || strings.Contains(err.Error(), "PRIVATE") || !connection.closed {
		t.Fatal("rejected subscription was retained or disclosed upstream payload", err)
	}
}
