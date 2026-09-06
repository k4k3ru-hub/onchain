package rest_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/funding_rates"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/fundings"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_book_details"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_book_orders"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_books"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/recent_trades"
)

func TestDeploymentMarketOperations(t *testing.T) {
	for _, tt := range []struct {
		name, url string
		new       func(rest.ClientParams) (*rest.Client, error)
	}{
		{"core", rest.CoreMainnetURL, rest.NewCoreClient}, {"robinhood", rest.RobinhoodMainnetURL, rest.NewRobinhoodClient}, {"legacy", rest.RobinhoodMainnetURL, rest.NewClient},
	} {
		t.Run(tt.name, func(t *testing.T) {
			seen := map[string]bool{}
			c, err := tt.new(rest.ClientParams{HTTPClient: httpFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Scheme+"://"+r.URL.Host != tt.url {
					t.Fatalf("wrong deployment: %s", r.URL)
				}
				seen[r.URL.Path] = true
				return response(`{"code":200}`), nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			if c.Markets == nil || c.Markets.OrderBooks == nil || c.Markets.OrderBookDetails == nil || c.Markets.OrderBookOrders == nil || c.Markets.RecentTrades == nil || c.Markets.Fundings == nil || c.Markets.FundingRates == nil {
				t.Fatal("missing composition")
			}
			ctx := context.Background()
			for _, send := range []func() error{
				func() error { _, e := c.Markets.OrderBooks.Send(ctx, order_books.Params{}); return e },
				func() error { _, e := c.Markets.OrderBookDetails.Send(ctx, order_book_details.Params{}); return e },
				func() error {
					_, e := c.Markets.OrderBookOrders.Send(ctx, order_book_orders.Params{Limit: 1})
					return e
				},
				func() error { _, e := c.Markets.RecentTrades.Send(ctx, recent_trades.Params{Limit: 1}); return e },
				func() error { _, e := c.Markets.Fundings.Send(ctx, fundings.Params{Resolution: "1h"}); return e },
				func() error { _, e := c.Markets.FundingRates.Send(ctx, funding_rates.Params{}); return e },
			} {
				if err := send(); err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range []string{"orderBooks", "orderBookDetails", "orderBookOrders", "recentTrades", "fundings", "funding-rates"} {
				if !seen["/api/v1/"+path] {
					t.Fatal("missing operation", path)
				}
			}
		})
	}
}

func TestDeploymentIsolation(t *testing.T) {
	transport := httpFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "mainnet.zklighter.elliot.ai":
			return response(`{"code":200,"order_books":[{"market_id":0,"symbol":"CORE"}]}`), nil
		case "api.rh.lighter.xyz":
			return response(`{"code":200,"order_books":[{"market_id":0,"symbol":"RH"}]}`), nil
		default:
			t.Errorf("unexpected host %s", r.URL.Host)
			return response(`{"code":400}`), nil
		}
	})
	core, err := rest.NewCoreClient(rest.ClientParams{HTTPClient: transport})
	if err != nil {
		t.Fatal(err)
	}
	rh, err := rest.NewRobinhoodClient(rest.ClientParams{HTTPClient: transport})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, tt := range []struct {
		client *rest.Client
		symbol string
	}{{core, "CORE"}, {rh, "RH"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := tt.client.Markets.OrderBooks.Send(context.Background(), order_books.Params{})
			if e != nil {
				t.Error(e)
				return
			}
			if len(r.OrderBooks) != 1 || r.OrderBooks[0].Symbol != tt.symbol {
				t.Error("cross-deployment data")
			}
		}()
	}
	wg.Wait()
	for _, newClient := range []func(rest.ClientParams) (*rest.Client, error){rest.NewCoreClient, rest.NewRobinhoodClient} {
		if _, err := newClient(rest.ClientParams{BaseURL: "https://other.example"}); err == nil {
			t.Fatal("accepted mismatched endpoint")
		}
		var nilHTTP httpFunc
		if _, err := newClient(rest.ClientParams{HTTPClient: nilHTTP}); err == nil {
			t.Fatal("accepted typed nil")
		}
	}
}
