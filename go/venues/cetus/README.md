# Cetus CLMM

## Independent reward inputs

`client.PoolRewards(ctx, poolAddress)` reads the pool reward manager through the
client's injected object reader. `clmm.ParsePoolRewards(object)` can instead decode
an already available full pool object without a network request. Both preserve
object version, manager update time, reward coin type and raw Q64 emissions.
They do not calculate APR: valuation, funding availability and the denominator
must be established separately. Missing reward managers return an error; explicit
empty reward lists remain empty. See [yield acquisition status](../YIELD.md).

## Retained stream quotes

An unknown tick deletion invalidates retained inputs and reports `tick_index`,
`checkpoint`, and `pool_id`. It is not silently ignored. The rejected checkpoint
becomes the minimum recovery checkpoint, so a subsequent `Warm` cannot install a
lagging baseline. The caller reconnects and performs a fresh capture; quote
snapshots become available again only after initialization succeeds.

`StateCache.RunRetained` initializes state and then applies streamed full pool and
field payloads. `QuoteRetainedPair` freezes the currently retained components and
calculates locally: no Trade/checkpoint alignment, waits, retries or RPC reads.
State notifications update retained inputs; the application chooses when to quote
(e.g. on Trade receipt), without publishing again for a late state notification.

A newer full pool payload replaces the pool even when its input version/digest
differs from the retained version. Supplied tick/bitmap values replace their
entries; entries absent from the notification remain as last received. This is a
receiver-side snapshot and does not assert atomic consistency across components.
Older pool notifications do not roll state back. Conflicting same-version digests,
malformed or deleted required inputs, incompatible configuration and disconnects
still invalidate the state session. These replacement rules must not be applied
to arithmetic deltas that require a continuous predecessor.

The checkpoint-validated `QuotePair` / simulation behavior documented below is a
separate path; it is not the retained Trade quote path.

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

Pool and tick reads are pinned to `atCheckpoint`. `Warm` reads only the central
compressed-tick interval and its two neighbors: each interval has 256 aligned
positions using the pool's `tick_spacing`. This is equivalent in span to EVM ±1
word, but Cetus stores u64 skip-list nodes rather than a bitmap. Keys are derived
from tick index + 443636, matching the [official SDK keyed lookup](https://github.com/CetusProtocol/cetus-clmm-sui-sdk/blob/main/src/modules/poolModule.ts).

At most 768 candidate keys are read in 16 sequential GraphQL batches of 50,
independent of the pool's total initialized tick count. Pool/checkpoint reads are
additional. Empty keys establish known-empty positions; every existing node in
range contributes its full tick data. Readers must provide
`DynamicUint64ValuesAtCheckpoint`; unsupported readers fail instead of falling
back to global pagination. `sui.RPCClient` supplies this capability.

`RunRetained` checks interval movement locally each second and refills in the
background with a 30-second timeout and 1–30 second failure backoff. Live pool and
in-range tick updates continue during capture. Installation replays buffered
transactions and rejects version/checkpoint regression; failure preserves live
inputs. Outside-range tick changes, including deletions, are ignored. Reconnect
uses the same bounded `Warm` capture. `QuoteRetainedPair` never issues RPC.

Verified empty intervals are usable for local quotes. A quote that would leave
coverage fails with `coverage=insufficient`; fixed coverage cannot guarantee all
quantities. No partial fill or automatic simulation fallback is returned. The
separate checkpoint-validated `QuotePair` may still use verified adjacent ticks;
its fallback is now the bounded window, never the full collection.
Local QuoteResult.AmountIn excludes FeeAmount, matching the Cetus Move fetcher.
Add the fee for gross input.

The checked-in vectors use the official TypeScript SDK 5.3.3 and a public pool
snapshot. They cover both directions, both amount modes and multiple tick crossings.
No Node or npm dependency is required to run the Go tests.
