package slipstream

import (
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/deployment"
)

type HTTPClientRPC interface {
	HTTPRPCClient
	LogFilterRPCClient
}

type ClientParams struct {
	HTTPRPCClient HTTPClientRPC
	WSRPCClient   WSRPCClient
	Deployment    deployment.Deployment
	SwapSources   []SwapSource
}

type Client struct {
	Factory        *FactoryClient
	Quoter         *QuoterClient
	PoolState      *PoolStateClient
	SwapFilter     *SwapFilterClient
	SwapSubscriber *SwapSubscriber
}

// NewClient composes the Slipstream protocol clients for one deployment.
//
// Parameters:
//   - params: RPC dependencies, deployment, and configured Swap sources.
//
// Returns:
//   - Composed Slipstream client.
//   - Composition error.
//
// Version:
//   - 2026-08-30: Added.
func NewClient(params ClientParams) (*Client, error) {
	if params.HTTPRPCClient == nil {
		return nil, fmt.Errorf("failed to create slipstream client: http_rpc_client=null")
	}
	if params.WSRPCClient == nil {
		return nil, fmt.Errorf("failed to create slipstream client: ws_rpc_client=null")
	}
	if params.Deployment.Factory == ([20]byte{}) {
		return nil, fmt.Errorf("failed to create slipstream client: deployment_factory=empty")
	}
	if params.Deployment.QuoterV2 == ([20]byte{}) {
		return nil, fmt.Errorf("failed to create slipstream client: deployment_quoter_v2=empty")
	}

	factory, err := NewFactoryClient(FactoryClientParams{
		RPC:     params.HTTPRPCClient,
		Factory: FactoryConfig{Address: params.Deployment.Factory},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create slipstream client: %w", err)
	}
	quoter, err := NewQuoterClient(QuoterClientParams{
		RPC:    params.HTTPRPCClient,
		Quoter: QuoterConfig{Address: params.Deployment.QuoterV2},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create slipstream client: %w", err)
	}
	poolState, err := NewPoolStateClient(PoolStateClientParams{RPC: params.HTTPRPCClient})
	if err != nil {
		return nil, fmt.Errorf("failed to create slipstream client: %w", err)
	}
	swapFilter, err := NewSwapFilterClient(SwapFilterClientParams{RPC: params.HTTPRPCClient, Sources: params.SwapSources})
	if err != nil {
		return nil, fmt.Errorf("failed to create slipstream client: %w", err)
	}
	swapSubscriber, err := NewSwapSubscriber(SwapSubscriberParams{RPC: params.WSRPCClient, Sources: params.SwapSources})
	if err != nil {
		return nil, fmt.Errorf("failed to create slipstream client: %w", err)
	}
	return &Client{
		Factory:        factory,
		Quoter:         quoter,
		PoolState:      poolState,
		SwapFilter:     swapFilter,
		SwapSubscriber: swapSubscriber,
	}, nil
}
