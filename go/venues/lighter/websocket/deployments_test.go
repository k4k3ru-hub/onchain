package websocket_test

import (
	"context"
	"encoding/json"
	"testing"

	ws "github.com/k4k3ru-hub/onchain/go/venues/lighter/websocket"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/websocket/subscriptions"
)

func TestExplicitDeployments(t *testing.T) {
	ctx := context.Background()
	var clients []*ws.Client
	for _, tt := range []struct {
		url string
		new func(ws.ClientParams) (*ws.Client, error)
	}{{ws.CoreMainnetURL, ws.NewCoreClient}, {ws.RobinhoodMainnetURL, ws.NewRobinhoodClient}} {
		conn := newConn()
		c, err := tt.new(ws.ClientParams{ReadOnly: true, Dialer: dialFunc(func(_ context.Context, url string) (ws.Connection, error) {
			if url != tt.url+"?readonly=true" {
				t.Errorf("wrong deployment: %s", url)
			}
			return conn, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
		if err := c.Connect(ctx); err != nil {
			t.Fatal(err)
		}
		for _, g := range []struct {
			client  *subscriptions.Client
			channel string
		}{{c.OrderBook, "order_book/0"}, {c.BBO, "ticker/0"}, {c.Trades, "trade/0"}, {c.MarketStats, "market_stats/0"}, {c.SpotMarketStats, "spot_market_stats/0"}} {
			if g.client == nil {
				t.Fatal("missing group")
			}
			if err := g.client.Subscribe(ctx, subscriptions.Params{MarketID: 0}); err != nil {
				t.Fatal(err)
			}
			var request struct {
				Channel string `json:"channel"`
			}
			if err := json.Unmarshal([]byte(<-conn.writes), &request); err != nil {
				t.Fatal(err)
			}
			if request.Channel != g.channel {
				t.Fatal(request.Channel)
			}
		}
		// Both deployments may publish the same market ID; each has its own receiver.
		conn.reads <- []byte(`{"type":"subscribed/order_book","channel":"order_book:0","order_book":{"code":200,"nonce":1,"asks":[],"bids":[]}}`)
		if _, err := c.Recv(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := clients[0].Close(); err != nil {
		t.Fatal(err)
	}
	// Closing Core must not close the independently composed Robinhood session.
	if err := clients[1].Trades.Unsubscribe(ctx, subscriptions.Params{}); err != nil {
		t.Fatal(err)
	}
	if err := clients[1].Close(); err != nil {
		t.Fatal(err)
	}
	for _, newClient := range []func(ws.ClientParams) (*ws.Client, error){ws.NewCoreClient, ws.NewRobinhoodClient} {
		if _, err := newClient(ws.ClientParams{EndpointURL: "wss://other.example"}); err == nil {
			t.Fatal("accepted mismatched endpoint")
		}
		var nilDialer dialFunc
		if _, err := newClient(ws.ClientParams{Dialer: nilDialer}); err == nil {
			t.Fatal("accepted typed nil")
		}
	}
}
