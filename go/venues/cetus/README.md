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

Simulation-based methods remain available alongside the local state cache below.

## Checkpoint-pinned local quotes

`clmm.NewStateCache(rpcClient, poolAddress, 30*time.Second)` composes local state
acquisition with the Sui GraphQL client. Call `cache.Warm(ctx)` with a bounded
initialization context before latency-sensitive quotes, then `cache.QuotePair`.
`cache.ObserveCheckpoint` forwards observed pool changes without blocking capture.
Both returned sides use one state; `StateTimestamp`, `PoolVersion` and `PoolDigest`
preserve provenance. A simulation pair exposes its actual input pool version and
digest when supplied by execution effects; its checkpoint is only observed head.

Pool and tick reads are pinned to `atCheckpoint`. Complete pages initialize the
cache; later reads can use verified adjacent ticks if both amounts remain inside
that interval. Changing/deleted neighbors and interval crossings fall back to
full capture. Consumers must budget for initial/full fetch latency. No partial
fills or automatic simulation fallback are returned. Local QuoteResult.AmountIn
excludes FeeAmount, matching the Cetus Move fetcher. Add the fee for gross input.

The checked-in vectors use the official TypeScript SDK 5.3.3 and a public pool
snapshot. They cover both directions, both amount modes and multiple tick crossings.
No Node or npm dependency is required to run the Go tests.
