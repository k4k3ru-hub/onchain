package markets_test

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/bbo"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/candles"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/funding_rates"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/list"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/order_book"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/trades"
)

type executorFunc func(context.Context, string, url.Values, any) error

func (f executorFunc) Get(c context.Context, p string, q url.Values, r any) error {
	return f(c, p, q, r)
}
func TestInjectedOperations(t *testing.T) {
	cause := errors.New("executor failure")
	calls := 0
	c, err := markets.NewClient(executorFunc(func(context.Context, string, url.Values, any) error { calls++; return cause }))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, send := range []func() error{
		func() error { _, e := c.List.Send(ctx, list.Params{}); return e },
		func() error { _, e := c.OrderBook.Send(ctx, order_book.Params{Market: "BTC-USD"}); return e },
		func() error { _, e := c.BBO.Send(ctx, bbo.Params{Market: "BTC-USD"}); return e },
		func() error { _, e := c.Trades.Send(ctx, trades.Params{Market: "BTC-USD"}); return e },
		func() error {
			_, e := c.Candles.Send(ctx, candles.Params{Market: "BTC-USD", Timeframe: "1m", To: 1788656400000000})
			return e
		},
		func() error { _, e := c.FundingRates.Send(ctx, funding_rates.Params{Market: "BTC-USD"}); return e },
	} {
		if err := send(); !errors.Is(err, cause) {
			t.Fatalf("lost executor error: %v", err)
		}
	}
	if calls != 6 {
		t.Fatal(calls)
	}
	for _, send := range []func() error{
		func() error { _, e := c.List.Send(nil, list.Params{}); return e },
		func() error { _, e := c.OrderBook.Send(ctx, order_book.Params{Market: "../account"}); return e },
		func() error { _, e := c.BBO.Send(ctx, bbo.Params{}); return e },
		func() error { _, e := c.Trades.Send(ctx, trades.Params{Market: "BTC-USD", Limit: 1001}); return e },
		func() error {
			_, e := c.Candles.Send(ctx, candles.Params{Market: "BTC-USD", Timeframe: "1m", To: 1788656400})
			return e
		},
		func() error { _, e := c.FundingRates.Send(ctx, funding_rates.Params{}); return e },
	} {
		if err := send(); err == nil || errors.Is(err, cause) {
			t.Fatalf("validation reached executor: %v", err)
		}
	}
	if calls != 6 {
		t.Fatal(calls)
	}
	var nilExecutor executorFunc
	if _, err := markets.NewClient(nilExecutor); err == nil {
		t.Fatal("typed nil accepted")
	}
}
