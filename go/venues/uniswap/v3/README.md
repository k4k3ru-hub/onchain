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
This describes `QuotePair`. `RunRetained` additionally preloads the current
bitmap word and one adjacent word on each side (three words, clipped at protocol
tick limits). When either adjacent word is unavailable after a streamed update,
the producer asynchronously captures a new window around the current on-chain
tick. Missing reference-quote tick details also trigger capture, including when
consumers use detached quote snapshots. Large jumps fetch the new neighborhood
directly rather than traversing the intervening words.

The live subscription and usable quote inputs remain active during capture.
Each candidate is read at one verified block, then buffered stream deltas after
that block are replayed before publication. The replay buffer is bounded to
4,096 pool logs; overflow, removed logs, and incompatible deltas still require
session recovery. Refreshes run one at a time, at least one second apart;
failed refreshes retain usable inputs and retry with delays up to 30 seconds.
Capture has a 30-second timeout. Initial capture errors return to the caller.

Session invalidation errors include a reason, pool, block number/hash and log
index, distinguishing removed logs, missing hashes, baseline/stream hash
mismatches, ordering violations, missing state/topics and unsupported events.
Callers should reset reconnect backoff after a sustained session. MarketHub
resets to one second after a session lasts at least 30 seconds; short sessions
continue exponential backoff up to 30 seconds. Session duration includes initial
capture, so this is a reconnect heuristic rather than proof of quote readiness.

Three words are the prefetch window, not a three-RPC total budget: core state,
reference-quote tick details and header verification are also required. Sparse
liquidity can require additional words for the reference quote, subject to the
same 64-contract-call cap. Each refresh replaces its old window rather than
mixing tick data from different blocks. Quote consumers never fetch missing
inputs; an uncovered quote fails until producer capture catches up.

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
one block; one contract call per subsequent refresh is removed. If HTTP `latest`
is behind the highest observed pool log, an active subscription with a known
non-removed block hash permits one explicit-number header lookup. Only an exact
number/hash match allows state reads at that observed block. Missing hashes,
removed logs and disconnects disable this fallback until a qualifying log arrives.
Unavailable or mismatching headers fail before contract reads; final canonical
hash verification and concurrent invalidation checks still apply. The fallback
adds at most one header request and has no internal retry loop. It does not
establish provider freshness beyond the observed block or guarantee recovery.

A delayed non-removed log matching both the verified snapshot block number and
hash is already represented by that block's terminal state and does not
invalidate the cache again. Other blocks, different hashes and removed logs
still invalidate; older blocks are conservatively treated as updates. TTL and
observation timestamps are unchanged. This common v3 optimization applies to
all configured chains, including Base. It does not guarantee fewer than two
RPC calls per quote on busy pools; deployment measurements remain necessary.
