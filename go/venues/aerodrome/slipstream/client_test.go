package slipstream

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/deployment"
)

type clientTestHTTPRPC struct {
	factoryTestRPC
}

// FilterLogs returns the configured log fixture.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func (*clientTestHTTPRPC) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

// TestNewClientComposesProtocolClients verifies new client composes protocol clients in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestNewClientComposesProtocolClients(t *testing.T) {
	t.Parallel()
	configuredDeployment, err := deployment.ByID(deployment.IDAerodromeBaseMainnetCurrent)
	if err != nil {
		t.Fatalf("ByID() error = %v", err)
	}
	httpRPC := &clientTestHTTPRPC{}
	wsRPC := &swapSubscriptionTestRPC{source: &swapTestSubscription{errs: make(chan error, 1), stopped: make(chan struct{})}}
	client, err := NewClient(ClientParams{
		HTTPRPCClient: httpRPC,
		WSRPCClient:   wsRPC,
		Deployment:    configuredDeployment,
		SwapSources:   swapTestSources(t)[:1],
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.Factory == nil || client.Quoter == nil || client.PoolState == nil || client.SwapFilter == nil || client.SwapSubscriber == nil {
		t.Fatalf("NewClient() = %+v", client)
	}
	if client.Factory.rpc != httpRPC || client.Quoter.rpc != httpRPC || client.PoolState.rpc != httpRPC || client.SwapFilter.rpc != httpRPC || client.SwapSubscriber.rpc != wsRPC {
		t.Fatal("NewClient() dependencies were not propagated")
	}
}

// TestNewClientRejectsMissingDependencies verifies new client rejects missing dependencies in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestNewClientRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	configuredDeployment, err := deployment.ByID(deployment.IDAerodromeBaseMainnetCurrent)
	if err != nil {
		t.Fatalf("ByID() error = %v", err)
	}
	httpRPC := &clientTestHTTPRPC{}
	wsRPC := &swapSubscriptionTestRPC{}
	sources := swapTestSources(t)[:1]
	tests := []ClientParams{
		{},
		{HTTPRPCClient: httpRPC, Deployment: configuredDeployment, SwapSources: sources},
		{HTTPRPCClient: httpRPC, WSRPCClient: wsRPC, SwapSources: sources},
		{HTTPRPCClient: httpRPC, WSRPCClient: wsRPC, Deployment: configuredDeployment},
	}
	for _, params := range tests {
		if _, err := NewClient(params); err == nil {
			t.Fatalf("NewClient(%+v) error = nil", params)
		}
	}
}

var _ HTTPClientRPC = (*clientTestHTTPRPC)(nil)
var _ HTTPRPCClient = (*clientTestHTTPRPC)(nil)
var _ LogFilterRPCClient = (*clientTestHTTPRPC)(nil)
