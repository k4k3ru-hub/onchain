package sui_test

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

// ExampleGRPCClient_SubscribeObjectStateScoped shows bounded pool-state consumption.
// This example is compile-checked; it does not connect during tests.
//
// Version:
//   - 2026-10-12: Added.
func ExampleGRPCClient_SubscribeObjectStateScoped() {
	// Compose the client once at the application boundary, then inject it into
	// consumers. Resolve actual pool/parent IDs from a verified pool object.
	consume := func(ctx context.Context, client *sui.GRPCClient, pool, tickParent sui.Address) error {
		sub, err := client.SubscribeObjectStateScoped(ctx, pool, sui.ObjectStateScope{
			ParentIDs: []sui.Address{tickParent},
		})
		if err != nil {
			return fmt.Errorf("failed to subscribe pool state: %w", err)
		}
		defer sub.Close()
		for {
			n, err := sub.Recv(ctx)
			if err != nil {
				// Withdraw calculation inputs and reinitialize before reconnecting.
				return fmt.Errorf("failed to receive pool state: %w", err)
			}
			if n.Effects == nil {
				// Progress only follows all preceding, hydrated object changes.
				continue
			}
			fmt.Println(n.Effects.Digest, len(n.ObjectChanges))
		}
	}
	_ = consume // Supply an application context and its composed client.
}
