# Momentum CLMM

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

The owning package is `github.com/k4k3ru-hub/onchain/go/venues/momentum/clmm`.
Migrated from momentum commit `8d0d216f5361bd6e9366b7e005b268dec9e82456`.
The original module is preserved; its types and the migrated package's types are
separate. Consumers must use the owning import consistently, including atomic
transaction construction. No dependency on the original module or root aliases
are introduced.

Compose the simulation/event client with `clmm.NewClient(deployment, rpc, grpc)`.
Compose the market-data state cache with:

```go
cache, err := clmm.NewStateCache(rpc, pool.Address, 30*time.Second)
if err != nil {
    return err
}
pair, err := cache.QuotePair(ctx, clmm.QuotePairParams{Bid: bidParams, Ask: askParams})
```

`rpc` is a Sui GraphQL client or an injected `clmm.StateReader`. The caller owns
transports and contexts. Observe live or reconciled pool changes with
`cache.ObserveCheckpoint(checkpoint)`. The independent simulation methods remain
available for comparison and atomic execution.

## State and quotes

- Capture the pool and lazily requested bitmap words/tick values at one explicit
  checkpoint. Keys use the pool package's `i32::I32` and signed little-endian BCS.
- Respect empty bitmap-word boundaries, signed tick compression, initialized tick
  liquidity changes and the explicit sqrt-price limit. Do not skip word boundaries:
  they contribute separate rounding steps even with unchanged liquidity.
- Use integer Q64 arithmetic. Momentum rounds the target-reaching/exact-output fee
  to the nearest integer; an unfinished exact-input step consumes the remaining
  budget after the recomputed input. This differs from Cetus's ceiling fee rule.
- `AmountIn` is gross, including `FeeAmount`. Never add the fee again. The local
  `FeeAmount` is total input fee, including protocol share; it is not the Move
  `get_state_fee_amount` LP-only field. Simulation methods only populate the fields
  they actually decode; additional local fields can be zero/nil in simulation results.
- Recheck the pool version/digest at each fresh indexed head. Reuse already-read
  words/ticks while that version remains unchanged, with bounded retention.
  A newly needed key is read at the original capture checkpoint, never mixed with
  a later checkpoint. Live observations impose a minimum acceptable checkpoint.
- Check pool `pause` and `trading_enabled` fields on capture. Missing keys follow
  contract defaults; malformed responses fail. Incomplete fills, missing initialized
  ticks, expired state, cancellation and more than 2048 steps return errors.
- `StateTimestamp` records indexed checkpoint time. Simulation `Checkpoint` alone
  is an observed head, not proof of its execution state; compare `PoolVersion` and
  `PoolDigest` from simulation input effects for parity.

Protocol reference: [Momentum v3 core](https://github.com/mmt-finance/v3-core/tree/667f446551b46054c2ac30d8a63df76420cb63f0),
especially `trade.move`, `swap_math.move`, `full_math_u64.move`, `tick_math.move`
and `tick_bitmap.move`. The local arithmetic is implemented without a new dependency.
