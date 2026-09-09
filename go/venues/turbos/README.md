# Turbos CLMM

## Retained stream quotes

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

Owning package: `github.com/k4k3ru-hub/onchain/go/venues/turbos/clmm`.
Migrated from Turbos commit `2821397deaa1ff8a12847934150eafaa28485e33`.
The original module remains intact. Consumers must consistently import the new
owning types; no root aliases or new production dependencies are introduced.

Compose the simulation/event client with `clmm.NewClient(deployment, rpc, grpc)`.
The market-data cache accepts the consumer-owned `clmm.StateReader` interface:

```go
cache, err := clmm.NewStateCache(rpc, pool.Address, 30*time.Second)
if err != nil {
    return err
}
pair, err := cache.QuotePair(ctx, clmm.QuotePairParams{Bid: bid, Ask: ask})
```

The caller owns transports and contexts. Feed observed pool-change checkpoints
to `cache.ObserveCheckpoint(checkpoint)`.

## State consistency

Pool, bitmap words and initialized ticks are read at an explicit GraphQL checkpoint.
Bitmap words live in `tick_map`; ticks are dynamic fields directly under the pool.
Both use signed little-endian `i32::I32` keys from the pool's original package.
Missing bitmap entries mean empty words; missing initialized ticks fail the quote.
Word boundaries are retained because each step contributes rounding.

Every pair checks indexed head age and pool version/digest. Unchanged versions
reuse lazy word/tick reads at their original capture checkpoint, with bounded
retention. Live checkpoints impose a minimum checkpoint and are checked again
before returning. This is checkpoint polling plus Swap observation, not a full
pool-object subscription. New keys are never read from a different state.

Integer Q64 calculations follow Turbos:
- token A input delta rounds up; token B input delta rounds to nearest;
- target-reaching/exact-output fee rounds to nearest;
- partial exact-input steps charge remaining budget minus recomputed input;
- protocol fee is floor(total step fee * fee_protocol / 1,000,000), summed per step;
- AmountIn already includes fees; do not add FeeAmount again;
- zero remaining liquidity stops traversal; incomplete fills are rejected.

The cache rejects locked pools, expired/invalidated snapshots, missing required
state, cancellation, unsupported amount ranges (local amounts must fit u64), and
more than 2048 steps. It is a market-data estimate, not a transaction executability
guarantee: deployment version gates, balances and transaction-specific constraints
still require simulation. Local failures do not trigger a simulation fallback.

Pair metadata includes StateTimestamp, PoolVersion and PoolDigest. Simulation
Checkpoint is an observed head, not an execution-state proof. Compare the exact
input version/digest for parity; the simulation methods remain independently
available for atomic routes.

## References

- [Turbos SDK](https://github.com/turbos-finance/turbos-clmm-sdk)
- [Published Turbos Move source](https://github.com/hackenproof-public/turbos-clmm-public/tree/ff0b03738d7e63b036563b8c6b3125e77b1c9737):
  pool, pool_fetcher, math_swap, math_sqrt_price, math_tick and full_math_u128.
