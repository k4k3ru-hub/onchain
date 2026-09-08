# Bluefin Spot

Owning package: `github.com/k4k3ru-hub/onchain/go/venues/bluefin/spot`.
Migrated from `bluefin` commit `f3ceb68ef69c42d88f4144007af6cc0cd721df78`.
The original module remains intact; consumers use the new owning types consistently,
including atomic transaction construction. No root aliases or production dependencies
are introduced.

Compose the simulation/event client with `spot.NewClient(deployment, rpc, grpc)`.
The market-data cache accepts the consumer-owned `spot.StateReader`:

```go
cache, err := spot.NewStateCache(rpc, pool.Address, 30*time.Second)
if err != nil {
    return err
}
pair, err := cache.QuotePair(ctx, spot.QuotePairParams{Bid: bid, Ask: ask})
```

Call `cache.ObserveCheckpoint(cp)` when a live/reconciled pool change arrives.
The caller owns transports, lifetime and request contexts. QuotePair computes both
sides independently without mutating the input state.

## State consistency and supported deployment

Pool, TickManager bitmap words and tick values share an explicit GraphQL checkpoint.
Words/ticks are fetched lazily. Each quote checks the fresh indexed head and the pool
version/digest; unchanged versions reuse already fetched keys. New keys are read at
the original capture checkpoint, never mixed across versions. Retention is bounded
by maxAge; observed pool checkpoints are checked before and after calculations.

Local quoting supports the original Bluefin mainnet pool package
`0x3492c874c1e3b3e2984e8c41b589e642d4d0a5d6459e5a9cfc2d52fd7c89c267`,
whose TickManager uses the original IntegerMate `i32::I32` type at
`0x714a63a0dba6da4f017b42d5d0fb78867f18bcde904868e51d951a5a6f5b7f57`.
Unrecognized pool packages fail before reading bitmap keys, avoiding silent empty
words caused by a mismatched integer package. Existing simulation APIs still accept
their configured deployments.

Integer Q64 arithmetic is implemented using the existing SDK arithmetic approach.
Bluefin-specific behavior is verified against the public interface and read-only
simulation; the published proprietary contract implementation is not copied here.

- Input amounts include the entire swap fee.
- FeeAmount is the LP portion; ProtocolFee is the separate protocol portion.
- Target-reaching and exact-output fees round up.
- Partial exact-input steps consume the available net budget without recomputing it
  from the rounded price.
- Protocol share is floored separately per step. Bitmap-word boundaries therefore
  remain part of the calculation, even when no tick is initialized.
- Signed tick crossings update liquidity. Missing initialized ticks, malformed
  values, expired or invalidated state, incomplete fills, out-of-range amounts,
  cancellation and more than 2048 steps return errors.
- Paused pools are rejected. The estimate is not a guarantee that a transaction
  passes global version, balance or transaction-specific checks.
- No automatic simulation or stale-price fallback is performed.

StateTimestamp is the indexed checkpoint time. Simulation's returned Checkpoint is
an observed head, not an execution-state proof. Use PoolVersion/PoolDigest from
simulation input effects to establish same-state parity.

References:
- [Bluefin interface](https://github.com/fireflyprotocol/bluefin-spot-contract-interface/tree/91ce930575b2aa3decab9b7e129b8002deba80c1)
- [IntegerMate type identity](https://github.com/CetusProtocol/integer-mate/blob/06660f704c4ac1d443ab62346a46b5b60d49df33/sui/Move.toml)
- [Bluefin published contract for verification](https://github.com/fireflyprotocol/bluefin-spot-contracts-public)
