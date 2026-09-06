package spot_test

import (
	"context"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/spot"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/spot/price"
)

func ExampleNewClient() {
	client, err := spot.NewClient(spot.ClientParams{})
	if err != nil {
		return
	}
	result, err := client.Price.Send(context.Background(), price.Params{
		ChainID:    4663,
		SellToken:  "0x39dBED3a2bd333467115dE45665cC57F813C4571",
		BuyToken:   "0x5fc5360D0400a0Fd4f2af552ADD042D716F1d168",
		SellAmount: "1000000000000000000", // 1 PONS in base units.
	})
	if err != nil {
		return
	}
	// Check result.All and result.Errors; Recommended may name another venue.
	// These are indicative amounts, not an order book or an executable quote.
	_ = result
}
