package websocket_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
	ws "github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket/subscriptions"
)

type fakeConn struct {
	writes  chan string
	reads   chan []byte
	closed  chan struct{}
	once    sync.Once
	pingErr error
}

func newConn() *fakeConn {
	return &fakeConn{writes: make(chan string, 10), reads: make(chan []byte, 10), closed: make(chan struct{})}
}
func (f *fakeConn) Read(ctx context.Context) ([]byte, error) {
	select {
	case b := <-f.reads:
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.closed:
		return nil, errors.New("closed")
	}
}
func (f *fakeConn) Write(ctx context.Context, b []byte) error {
	select {
	case f.writes <- string(b):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (f *fakeConn) Ping(context.Context) error { return f.pingErr }
func (f *fakeConn) Close() error               { f.once.Do(func() { close(f.closed) }); return nil }

type dialFunc func(context.Context, string) (ws.Connection, error)

func (f dialFunc) Dial(c context.Context, u string) (ws.Connection, error) { return f(c, u) }

func TestCompositionLifecycleAndSubscriptions(t *testing.T) {
	conn := newConn()
	c, err := ws.NewClient(ws.ClientParams{Dialer: dialFunc(func(ctx context.Context, u string) (ws.Connection, error) {
		if u != ws.RobinhoodMainnetURL {
			t.Fatal(u)
		}
		return conn, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Trades.Subscribe(ctx, subscriptions.Params{Market: "BTC-USD"}); err == nil {
		t.Fatal("sent before connect")
	}
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Connect(ctx); err == nil {
		t.Fatal("double connect")
	}
	for _, tt := range []struct {
		group   *subscriptions.Client
		channel string
	}{{c.OrderBook, "l2Orderbook"}, {c.OrderBookUpdates, "l2OrderbookUpdates"}, {c.BBO, "bbo"}, {c.Trades, "trades"}, {c.Markets, "markets"}, {c.OraclePrices, "oraclePrices"}, {c.PredictedFunding, "predictedFunding"}} {
		if tt.group == nil {
			t.Fatal("missing composition")
		}
		p := subscriptions.Params{Market: "BTC-USD"}
		suffix := `,"id":"BTC-USD"`
		if tt.channel == "markets" {
			p.Market = ""
			suffix = ""
		}
		if err := tt.group.Subscribe(ctx, p); err != nil {
			t.Fatal(err)
		}
		if got := <-conn.writes; got != `{"type":"subscribe","channel":"`+tt.channel+`"`+suffix+`}` {
			t.Fatal(got)
		}
		if err := tt.group.Unsubscribe(ctx, p); err != nil {
			t.Fatal(err)
		}
		if got := <-conn.writes; got != `{"type":"unsubscribe","channel":"`+tt.channel+`"`+suffix+`}` {
			t.Fatal(got)
		}
	}
	conn.reads <- []byte(`{"type":"degraded","channel":"markets","reason":"snapshot_unavailable","retryAfterMs":5000}`)
	if m, err := c.Recv(ctx); err != nil || m.RetryAfterMS != 5000 {
		t.Fatalf("degraded: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestKeepaliveFailure(t *testing.T) {
	cause := errors.New("ping failure")
	conn := newConn()
	conn.pingErr = cause
	client, err := ws.NewClient(ws.ClientParams{PingInterval: time.Millisecond, Dialer: dialFunc(func(context.Context, string) (ws.Connection, error) { return conn, nil })})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-conn.closed:
	case <-time.After(time.Second):
		t.Fatal("keepalive did not close")
	}
	if err := client.Close(); !errors.Is(err, cause) {
		t.Fatalf("lost ping error: %v", err)
	}
}

func TestDefaultTransportCancellation(t *testing.T) {
	serverErrors := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := gorilla.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverErrors <- err
			return
		}
		defer func() { serverErrors <- c.Close() }()
		if err := c.WriteMessage(gorilla.TextMessage, []byte(`{"type":"connected"}`)); err != nil {
			t.Error(err)
			return
		}
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	client, err := ws.NewClient(ws.ClientParams{EndpointURL: "ws" + strings.TrimPrefix(server.URL, "http"), PingInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m, err := client.Recv(context.Background()); err != nil || m.Type != "connected" {
		t.Fatalf("recv: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.Recv(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation: %v", err)
	}
	// A concurrent ping may retain the first terminal cause; Recv must still
	// expose its own cancellation above.
	if err := client.Close(); err == nil {
		t.Fatal("close lost terminal failure")
	}
	select {
	case err := <-serverErrors:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server stuck")
	}
}
