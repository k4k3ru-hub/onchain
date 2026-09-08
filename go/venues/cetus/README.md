# Cetus CLMM

The owning Go package is `github.com/k4k3ru-hub/onchain/go/venues/cetus/clmm`.
It includes deployment configuration, pool parsing, simulated exact-input,
exact-output and paired quotes, historical/live swaps, and atomic transaction
construction. Compose with `clmm.NewClient(deployment, rpcClient, grpcClient)`;
Sui clients remain in `github.com/k4k3ru-hub/onchain/go/sui`.

```go
import (
    cetus "github.com/k4k3ru-hub/onchain/go/venues/cetus/clmm"
    sui "github.com/k4k3ru-hub/onchain/go/sui"
)

func newCetus(deployment cetus.Deployment, rpc *sui.RPCClient, grpc *sui.GRPCClient) (*cetus.Client, error) {
    return cetus.NewClient(deployment, rpc, grpc)
}
```

Migrated from cetus commit `05fb796c18e4d7ef458b521f8127f6dece3ae70c`.
The original module is preserved, but the two packages own distinct Go types;
consumers must consistently use the new path. No original Cetus module dependency
or root aliases are introduced. Existing composition and injected-fake operation
tests accompany the implementation.

This migration retains simulation-based quotes. Local state synchronization,
tick traversal, quote parity and MarketHub cache integration are subsequent work.
