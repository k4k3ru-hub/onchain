package api_test

import (
	"context"
	"fmt"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui"
	"github.com/k4k3ru-hub/onchain/go/venues/cetus/api"
)

// ExampleNewClient demonstrates one bounded page read with the owning API package.
//
// Version:
//   - 2026-09-14: Added.
func ExampleNewClient() {
	client, err := api.NewClient(api.Config{})
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	page, err := client.Pools.List(ctx, api.ListParams{Limit: 20, Offset: 0})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, pool := range page.Pools {
		if pool.APR != nil && pool.APR.FeeAPR24h != nil {
			fmt.Println(pool.Address, "24h fee APR (ratio):", pool.APR.FeeAPR24h.String())
		}
	}
}

// ExamplePoolsClient_Get demonstrates fetching APR for one pool ID.
//
// Version:
//   - 2026-09-14: Added.
func ExamplePoolsClient_Get() {
	client, err := api.NewClient(api.Config{})
	if err != nil {
		fmt.Println(err)
		return
	}
	poolID, err := sui.ParseAddress("0x51e883ba7c0b566a26cbc8a94cd33eb0abd418a77cc1e60ad22fd9b1f29cd2ab")
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := client.Pools.Get(ctx, poolID)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, stats := range pool.Stats {
		if stats.DateType == "24H" && stats.APR != nil {
			fmt.Println(pool.Pool, "24h fee APR (ratio):", stats.APR.String())
		}
	}
}
