package rest

import "fmt"

const CoreMainnetURL = "https://mainnet.zklighter.elliot.ai"

// NewCoreClient composes a client for Lighter Core mainnet using explicit dependencies.
// An endpoint override must equal CoreMainnetURL; use NewClient for custom deployments.
//
// Version:
//   - 2026-09-06: Added.
func NewCoreClient(p ClientParams) (*Client, error) {
	return newDeploymentClient(p, CoreMainnetURL)
}

// NewRobinhoodClient composes a client for the independent Robinhood Chain instance.
// An endpoint override must equal RobinhoodMainnetURL; use NewClient for custom deployments.
//
// Version:
//   - 2026-09-06: Added.
func NewRobinhoodClient(p ClientParams) (*Client, error) {
	return newDeploymentClient(p, RobinhoodMainnetURL)
}

func newDeploymentClient(p ClientParams, endpoint string) (*Client, error) {
	if p.BaseURL != "" && p.BaseURL != endpoint {
		return nil, fmt.Errorf("failed to create lighter rest client: deployment_endpoint=invalid")
	}
	p.BaseURL = endpoint
	client, err := NewClient(p)
	if err != nil {
		return nil, fmt.Errorf("failed to compose lighter deployment: %w", err)
	}
	return client, nil
}
