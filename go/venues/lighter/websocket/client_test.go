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
	ws "github.com/k4k3ru-hub/onchain/go/venues/lighter/websocket"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/websocket/subscriptions"
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
	client, err := ws.NewClient(ws.ClientParams{ReadOnly: true, Dialer: dialFunc(func(ctx context.Context, endpoint string) (ws.Connection, error) {
		if endpoint != ws.RobinhoodMainnetURL+"?readonly=true" {
			t.Errorf("endpoint %s", endpoint)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("unbounded dial")
		}
		return conn, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if client.OrderBook == nil || client.BBO == nil || client.Trades == nil || client.MarketStats == nil || client.SpotMarketStats == nil {
		t.Fatal("missing composition")
	}
	ctx := context.Background()
	if err := client.OrderBook.Subscribe(ctx, subscriptions.Params{}); err == nil {
		t.Fatal("sent before connect")
	}
	if err := client.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Connect(ctx); err == nil {
		t.Fatal("double connect")
	}
	for _, entry := range []struct {
		group   *subscriptions.Client
		channel string
		all     bool
	}{{client.OrderBook, "order_book/0", false}, {client.BBO, "ticker/0", false}, {client.Trades, "trade/0", false}, {client.MarketStats, "market_stats/all", true}, {client.SpotMarketStats, "spot_market_stats/all", true}} {
		p := subscriptions.Params{All: entry.all}
		if err := entry.group.Subscribe(ctx, p); err != nil {
			t.Fatal(err)
		}
		if got := <-conn.writes; got != `{"type":"subscribe","channel":"`+entry.channel+`"}` {
			t.Fatal(got)
		}
		if err := entry.group.Unsubscribe(ctx, p); err != nil {
			t.Fatal(err)
		}
		if got := <-conn.writes; got != `{"type":"unsubscribe","channel":"`+entry.channel+`"}` {
			t.Fatal(got)
		}
	}
	if err := client.OrderBook.Subscribe(ctx, subscriptions.Params{All: true}); err == nil {
		t.Fatal("accepted all books")
	}
	conn.reads <- []byte(`{"type":"ping"}`)
	if _, err := client.Recv(ctx); err != nil {
		t.Fatal(err)
	}
	if got := <-conn.writes; got != `{"type":"pong"}` {
		t.Fatal(got)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
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
