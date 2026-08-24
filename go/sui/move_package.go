package sui

import (
	"context"
	"fmt"
	"strings"
)

type MovePackage struct {
	Address Address
	Version uint64
	Digest  ObjectDigest
	Modules []string
}

// MovePackage returns the latest version of a Sui Move package.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - address: package address.
//
// Returns:
//   - SDK-owned Move package metadata.
//   - Retrieval or validation error.
//
// Version:
//   - 2026-08-23: Added.
func (c *RPCClient) MovePackage(ctx context.Context, address Address) (*MovePackage, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to get sui move package: rpc_client=null")
	}
	if c.caller == nil {
		return nil, fmt.Errorf("failed to get sui move package: package_provider=null")
	}
	if address.IsZero() {
		return nil, fmt.Errorf("failed to get sui move package: address=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result struct {
		Package *struct {
			Address string `json:"address"`
			Version uint64 `json:"version"`
			Digest  string `json:"digest"`
			Modules struct {
				Nodes []struct {
					Name string `json:"name"`
				} `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"modules"`
		} `json:"package"`
	}
	query := fmt.Sprintf(`query { package(address: %q) { address version digest modules(first: 100) { nodes { name } pageInfo { hasNextPage endCursor } } } }`, address.String())
	if err := c.caller.query(ctx, query, &result); err != nil {
		return nil, fmt.Errorf("failed to get sui move package: %w", err)
	}
	if result.Package == nil {
		return nil, fmt.Errorf("failed to get sui move package: package=null")
	}
	returnedAddress, err := ParseAddress(result.Package.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get sui move package: %w", err)
	}
	if returnedAddress != address {
		return nil, fmt.Errorf("failed to get sui move package: address=mismatch")
	}
	digest, err := ParseObjectDigest(result.Package.Digest)
	if err != nil {
		return nil, fmt.Errorf("failed to get sui move package: %w", err)
	}
	modules := make([]string, 0, len(result.Package.Modules.Nodes))
	for _, module := range result.Package.Modules.Nodes {
		name := strings.TrimSpace(module.Name)
		if name == "" {
			return nil, fmt.Errorf("failed to get sui move package: module_name=empty")
		}
		modules = append(modules, name)
	}
	if result.Package.Modules.PageInfo.HasNextPage {
		return nil, fmt.Errorf("failed to get sui move package: modules=too_long max_page_size=100")
	}
	return &MovePackage{Address: returnedAddress, Version: result.Package.Version, Digest: digest, Modules: modules}, nil
}
