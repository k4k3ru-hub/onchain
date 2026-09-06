package websocket_test

import (
	"context"
	"errors"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket/protocol"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket/subscriptions"
)

func watch(ctx context.Context, market string, consume func(*protocol.Message) error) (err error) {
	client, err := websocket.NewClient(websocket.ClientParams{})
	if err != nil {
		return err
	}
	if err := client.Connect(ctx); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, client.Close()) }()
	if err := client.OrderBookUpdates.Subscribe(ctx, subscriptions.Params{
		Market:  market,
		NLevels: 20,
	}); err != nil {
		return err
	}
	for {
		message, err := client.Recv(ctx)
		if err != nil {
			return err
		}
		if err := consume(message); err != nil {
			return err
		}
	}
}
