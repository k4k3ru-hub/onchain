package clmm

import (
	"fmt"
	"strings"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type Deployment struct {
	Package        onchainSui.Address
	PublishedAt    onchainSui.Address
	FetcherPackage onchainSui.Address
	GlobalConfig   onchainSui.ObjectInput
	Clock          onchainSui.ObjectInput
	PoolModule     string
	FetcherModule  string
}

// Validate validates a Cetus CLMM deployment.
//
// Returns:
//   - Validation error.
//
// Version:
//   - 2026-08-31: Required the separate Cetus integrate package used by quote fetchers.
//   - 2026-08-30: Added.
func (d Deployment) Validate() error {
	if d.Package.IsZero() {
		return fmt.Errorf("failed to validate cetus clmm deployment: package=empty")
	}
	if d.PublishedAt.IsZero() {
		return fmt.Errorf("failed to validate cetus clmm deployment: published_at=empty")
	}
	if d.FetcherPackage.IsZero() {
		return fmt.Errorf("failed to validate cetus clmm deployment: fetcher_package=empty")
	}
	if d.GlobalConfig.Address.IsZero() || d.GlobalConfig.Version == 0 {
		return fmt.Errorf("failed to validate cetus clmm deployment: global_config=invalid")
	}
	if d.Clock.Address.IsZero() || d.Clock.Version == 0 {
		return fmt.Errorf("failed to validate cetus clmm deployment: clock=invalid")
	}
	if strings.TrimSpace(d.PoolModule) == "" {
		return fmt.Errorf("failed to validate cetus clmm deployment: pool_module=empty")
	}
	if strings.TrimSpace(d.FetcherModule) == "" {
		return fmt.Errorf("failed to validate cetus clmm deployment: fetcher_module=empty")
	}
	return nil
}
