package evm_test

import (
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/core"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

// ExampleResolveChainID_robinhood demonstrates Robinhood Chain resolution.
//
// Version:
//   - 2026-09-06: Added.
func ExampleResolveChainID_robinhood() {
	id, err := evm.ResolveChainID(core.ChainRobinhood, core.NetworkMainnet)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(id)
	// Output: 4663
}
