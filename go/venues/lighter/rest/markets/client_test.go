package markets_test

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/funding_rates"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/fundings"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_book_details"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_book_orders"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_books"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/recent_trades"
)

type executorFunc func(context.Context, string, url.Values, any) error

func (f executorFunc) Get(c context.Context, p string, q url.Values, r any) error {
	return f(c, p, q, r)
}

func TestInjectedOperations(t *testing.T) {
	sentinel := errors.New("executor failure")
	calls := 0
	c, err := markets.NewClient(executorFunc(func(ctx context.Context, path string, q url.Values, result any) error { calls++; return sentinel }))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for name, send := range map[string]func() error{
		"books":    func() error { _, e := c.OrderBooks.Send(ctx, order_books.Params{}); return e },
		"details":  func() error { _, e := c.OrderBookDetails.Send(ctx, order_book_details.Params{}); return e },
		"orders":   func() error { _, e := c.OrderBookOrders.Send(ctx, order_book_orders.Params{Limit: 250}); return e },
		"trades":   func() error { _, e := c.RecentTrades.Send(ctx, recent_trades.Params{Limit: 100}); return e },
		"fundings": func() error { _, e := c.Fundings.Send(ctx, fundings.Params{Resolution: "1h"}); return e },
		"rates":    func() error { _, e := c.FundingRates.Send(ctx, funding_rates.Params{}); return e },
	} {
		t.Run(name, func(t *testing.T) {
			if err := send(); !errors.Is(err, sentinel) {
				t.Fatalf("lost executor error: %v", err)
			}
		})
	}
	if calls != 6 {
		t.Fatalf("calls=%d", calls)
	}
	for _, send := range []func() error{
		func() error { _, e := c.OrderBooks.Send(ctx, order_books.Params{Filter: "invalid"}); return e },
		func() error { _, e := c.OrderBookOrders.Send(ctx, order_book_orders.Params{Limit: 251}); return e },
		func() error { _, e := c.RecentTrades.Send(ctx, recent_trades.Params{Limit: 101}); return e },
		func() error { _, e := c.RecentTrades.Send(ctx, recent_trades.Params{MarketID: -1, Limit: 1}); return e },
		func() error { _, e := c.Fundings.Send(ctx, fundings.Params{Resolution: "1m"}); return e },
		func() error {
			_, e := c.Fundings.Send(ctx, fundings.Params{Resolution: "1h", StartTimestamp: 2, EndTimestamp: 1})
			return e
		},
	} {
		if err := send(); err == nil || errors.Is(err, sentinel) {
			t.Fatalf("validation failed: %v", err)
		}
	}
	if calls != 6 {
		t.Fatal("invalid request reached executor")
	}
	var nilExecutor executorFunc
	if _, err := markets.NewClient(nilExecutor); err == nil {
		t.Fatal("accepted typed nil")
	}
}
