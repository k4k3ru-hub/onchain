package rest_test

import (
	"context"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_books"
)

func coreMarkets(ctx context.Context) (*order_books.Result, error) {
	c, err := rest.NewCoreClient(rest.ClientParams{})
	if err != nil {
		return nil, err
	}
	return c.Markets.OrderBooks.Send(ctx, order_books.Params{})
}
func robinhoodMarkets(ctx context.Context) (*order_books.Result, error) {
	c, err := rest.NewRobinhoodClient(rest.ClientParams{})
	if err != nil {
		return nil, err
	}
	return c.Markets.OrderBooks.Send(ctx, order_books.Params{})
}
