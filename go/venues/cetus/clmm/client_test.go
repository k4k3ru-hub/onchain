package clmm

import (
	"context"
	"encoding/json"
	"testing"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type clientTestProvider struct {
	object     *onchainSui.Object
	eventPage  onchainSui.EventPage
	eventQuery onchainSui.EventQuery
}

func (p *clientTestProvider) Object(context.Context, onchainSui.Address) (*onchainSui.Object, error) {
	return p.object, nil
}

func (p *clientTestProvider) Events(_ context.Context, query onchainSui.EventQuery) (onchainSui.EventPage, error) {
	p.eventQuery = query
	return p.eventPage, nil
}

func (p *clientTestProvider) SubscribeEvents(context.Context, onchainSui.LiveEventFilter) (*onchainSui.EventSubscription, error) {
	return nil, nil
}

// TestComposeClient verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestComposeClient(t *testing.T) {
	provider := &clientTestProvider{}
	simulator := &quoteTestSimulator{}
	client, err := composeClient(testDeployment(), provider, provider, provider, simulator)
	if err != nil {
		t.Fatalf("composeClient() returned an unexpected error: %v", err)
	}
	if client.objects == nil || client.events == nil || client.subscriber == nil || client.quoter == nil {
		t.Fatalf("composeClient() did not compose all dependencies: %+v", client)
	}
}

// TestClientSwaps verifies Cetus CLMM behavior.
//
// Version:
//   - 2026-09-08: Migrated to onchain.
func TestClientSwaps(t *testing.T) {
	pool, _ := onchainSui.ParseAddress("0x9")
	provider := &clientTestProvider{eventPage: onchainSui.EventPage{
		Events: []onchainSui.Event{{
			Checkpoint:     6,
			SequenceNumber: 7,
			Type:           testDeployment().Package.String() + "::pool::SwapEvent",
			JSON:           json.RawMessage(`{"pool":"` + pool.String() + `","atob":true,"amount_in":"100","amount_out":"99","fee_amount":"1","before_sqrt_price":"10","after_sqrt_price":"11"}`),
		}},
		HasNextPage: true,
		NextCursor:  "cursor",
	}}
	client, err := composeClient(testDeployment(), provider, provider, provider, &quoteTestSimulator{})
	if err != nil {
		t.Fatalf("composeClient() returned an unexpected error: %v", err)
	}
	page, err := client.Swaps(context.Background(), SwapQuery{Pool: &pool, First: 20})
	if err != nil {
		t.Fatalf("Swaps() returned an unexpected error: %v", err)
	}
	if len(page.Swaps) != 1 || page.Swaps[0].Checkpoint != 6 || page.Swaps[0].SequenceNumber != 7 || page.Swaps[0].AmountOut != 99 || !page.HasNextPage || page.NextCursor != "cursor" {
		t.Fatalf("Swaps() = %+v", page)
	}
	if provider.eventQuery.Filter.Type != testDeployment().Package.String()+"::pool::SwapEvent" || provider.eventQuery.First != 20 {
		t.Fatalf("Events() query = %+v", provider.eventQuery)
	}
}

func testDeployment() Deployment {
	packageAddress, _ := onchainSui.ParseAddress("0x1")
	fetcherPackage, _ := onchainSui.ParseAddress("0x5")
	configAddress, _ := onchainSui.ParseAddress("0x2")
	clockAddress, _ := onchainSui.ParseAddress("0x6")
	return Deployment{
		Package:        packageAddress,
		PublishedAt:    packageAddress,
		FetcherPackage: fetcherPackage,
		GlobalConfig:   onchainSui.ObjectInput{Address: configAddress, Version: 1},
		Clock:          onchainSui.ObjectInput{Address: clockAddress, Version: 1},
		PoolModule:     "pool",
		FetcherModule:  "fetcher_script",
	}
}
