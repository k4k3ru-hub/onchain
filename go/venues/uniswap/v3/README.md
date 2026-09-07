# Uniswap v3

This package owns the migrated Uniswap v3 HTTP/WS clients, QuoterV2 calls,
Swap parsing/filtering/subscriptions, slot0 reads, pool keys, and deployment data.
The original clients were migrated from `uniswap/go/v3`; imports use
`github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3`.

Import `.../v3/protocol` for pool keys and currencies and `.../v3/deployment` for
deployment metadata. There are no root aliases or dependencies on the old v3
package. Existing users of `uniswap/go/v3` can continue using that package;
its named types are distinct and must not be mixed with the new package types.
Uniswap v4 remains in its existing repository.

## Composition

```go
import (
    v3 "github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3"
    "github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
    "github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/deployment"
)
```

Use `v3.NewHTTPClient(v3.HTTPClientParams{RPC: httpRPC, Factory: factory})`
and `v3.NewWSClient(v3.WSClientParams{RPC: wsRPC, Factory: factory})` for independent
lifecycles, or `v3.NewClient` to compose both. RPC interfaces remain injectable.
`factory` is a `v3.FactoryConfig`; its pool keys are `protocol.PoolKey` values,
and `deployment.ByChainID` supplies supported deployment addresses.

Run `go test ./venues/uniswap/v3/...` and `go vet ./venues/uniswap/v3/...`
from `onchain/go`. Existing constructor, quote, event, and deployment tests moved
with the implementation. Existing quote methods still call QuoterV2.

## Local state quotes

Compose `NewStateCache(httpClient, stateRPC, poolAddress, maxAge)` per pool.
Run `cache.Run(ctx, wsRPC)` in a supervised goroutine and call
`cache.QuotePair(ctx, baseAmount, baseIsToken0)` for an exact-input bid and
reverse exact-output ask. These are independent simulations on one snapshot.
The returned amounts include the pool fee and price impact.

All contract reads use one block number; canonical hash verification follows
new reads. Pool logs (including liquidity changes and removed logs), reconnects,
and disconnects invalidate the snapshot. Without a live subscription each pair
refreshes. TTL bounds reuse when a notification is lost; this is not a guarantee
that every event was received. `ObservedAt` is the snapshot acquisition time and
is preserved on reuse; it is not the block timestamp.

Only bitmap words and initialized ticks encountered by the simulation are read.
Each pair permits 64 contract calls, plus header reads; each direction permits
2048 steps. Exhausted budgets, insufficient liquidity, changed snapshots and
reorgs return errors without partial quotes. Cache hits need no RPC; active
pools can invalidate every poll and cost more RPC than two Quoter calls.

Integer rounding follows [SwapMath](https://github.com/Uniswap/v3-core/blob/main/contracts/libraries/SwapMath.sol),
[SqrtPriceMath](https://github.com/Uniswap/v3-core/blob/main/contracts/libraries/SqrtPriceMath.sol)
and [TickMath](https://github.com/Uniswap/v3-core/blob/main/contracts/libraries/TickMath.sol).
Tests cover reference numeric vectors, both directions, tick crossing, cache
reuse, expiry, invalidation and concurrency. Live same-block Quoter parity and
provider load remain deployment validation tasks.

## Avoiding redundant refresh work

After the first successfully verified pair, the cache retains `tickSpacing`.
This property is immutable in the [official pool implementation](https://github.com/Uniswap/v3-core/blob/main/contracts/UniswapV3Pool.sol).
Later snapshots still read mutable slot0, liquidity, bitmap words and ticks at
one block; one contract call per subsequent refresh is removed. An HTTP head
behind the highest observed pool log is rejected before any contract reads.

A delayed non-removed log matching both the verified snapshot block number and
hash is already represented by that block's terminal state and does not
invalidate the cache again. Other blocks, different hashes and removed logs
still invalidate; older blocks are conservatively treated as updates. TTL and
observation timestamps are unchanged. This common v3 optimization applies to
all configured chains, including Base. It does not guarantee fewer than two
RPC calls per quote on busy pools; deployment measurements remain necessary.
