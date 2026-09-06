package rest_test

import (
	"context"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/list"
)

func listMarkets(ctx context.Context) (*list.Result, error) {
	client, err := rest.NewClient(rest.ClientParams{})
	if err != nil {
		return nil, err
	}
	return client.Markets.List.Send(ctx, list.Params{})
}
