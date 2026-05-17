//
// network.go
//
package onchain

import (
    "fmt"
)

type Network string

const (
    NetworkMainnet Network = "mainnet"
    NetworkTestnet Network = "testnet"
    NetworkDevnet  Network = "devnet"
	NetworkSepolia Network = "sepolia"
	NetworkHolesky Network = "holesky"
)


//
// Check whether network is valid.
//
// Version:
//   - 2026-05-17: Added.
//
func (n Network) IsValid() bool {
    switch n {
    case NetworkMainnet,
        NetworkTestnet,
        NetworkDevnet,
        NetworkSepolia,
        NetworkHolesky:
        return true
    default:
        return false
    }
}


//
// Validate network.
//
// Version:
//   - 2026-05-17: Added.
//
func (n Network) Validate() error {
    if !n.IsValid() {
        return fmt.Errorf("invalid parameter: network=%q", truncateRunes(string(n), 16))
    }
    return nil
}


//
// Convert network to string.
//
// Version:
//   - 2026-05-17: Added.
//
func (n Network) String() string {
    return string(n)
}

