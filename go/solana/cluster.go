package solana

import (
	"context"
	"fmt"
)

type Cluster string

const (
	ClusterMainnetBeta Cluster = "mainnet-beta"
	ClusterTestnet     Cluster = "testnet"
	ClusterDevnet      Cluster = "devnet"
)

var clusterGenesisHashes = map[Cluster]string{
	ClusterMainnetBeta: "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d",
	ClusterTestnet:     "4uhcVJyU9pJkvQyS88uRDiswHXSCkY3zQawwpjk2NsNY",
	ClusterDevnet:      "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG",
}

type genesisHashProvider interface {
	getGenesisHash(context.Context) (Hash, error)
}

// String returns the Solana cluster name.
func (c Cluster) String() string { return string(c) }

// Validate validates a Solana cluster.
func (c Cluster) Validate() error {
	if _, ok := clusterGenesisHashes[c]; !ok {
		return fmt.Errorf("failed to validate solana cluster: cluster=invalid")
	}
	return nil
}

// ResolveCluster resolves a Solana cluster from its genesis hash.
//
// Parameters:
//   - genesisHash: cluster genesis hash.
//
// Returns:
//   - Resolved cluster.
//   - Resolution error.
//
// Version:
//   - 2026-08-22: Added.
func ResolveCluster(genesisHash Hash) (Cluster, error) {
	if genesisHash.IsZero() {
		return "", fmt.Errorf("failed to resolve solana cluster: genesis_hash=empty")
	}
	for cluster, value := range clusterGenesisHashes {
		if genesisHash.String() == value {
			return cluster, nil
		}
	}
	return "", fmt.Errorf("failed to resolve solana cluster: genesis_hash=invalid")
}

// Cluster returns the cluster identified by the RPC node's genesis hash.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//
// Returns:
//   - Resolved Solana cluster.
//   - Retrieval or resolution error.
//
// Version:
//   - 2026-08-22: Added.
func (c *RPCClient) Cluster(ctx context.Context) (Cluster, error) {
	if c == nil {
		return "", fmt.Errorf("failed to get solana cluster: rpc_client=null")
	}
	if c.genesisHashProvider == nil {
		return "", fmt.Errorf("failed to get solana cluster: genesis_hash_provider=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	hash, err := c.genesisHashProvider.getGenesisHash(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get solana cluster: %w", err)
	}
	cluster, err := ResolveCluster(hash)
	if err != nil {
		return "", fmt.Errorf("failed to get solana cluster: %w", err)
	}
	return cluster, nil
}
