package evm_test

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/evm"
)

func ExampleWSClient_SubscribeHeaders() {
	consume := func(ctx context.Context, client *evm.WSClient) error {
		subscription, err := client.SubscribeHeaders(ctx)
		if err != nil {
			return fmt.Errorf("failed to watch pool headers: %w", err)
		}
		defer subscription.Close()
		header, err := subscription.Recv(ctx)
		if err != nil {
			return fmt.Errorf("failed to watch pool headers: %w", err)
		}
		// Match a pool log's BlockHash to header.Hash before using Timestamp.
		fmt.Println(header.Number, header.Hash, header.Timestamp)
		return nil
	}
	_ = consume // Supply the application's existing WebSocket client and context.
}
