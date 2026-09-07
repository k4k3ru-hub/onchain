# Aerodrome

Go protocol clients for Aerodrome Slipstream.

## Supported deployments

- Base mainnet current Slipstream deployment
- Base mainnet initial/legacy Slipstream deployment

The legacy deployment remains supported because active pools retain liquidity and Swap activity. Deployment generation and current pool Swap fee are independent; retrieve the latter through `FactoryClient.GetSwapFee`.

## Owning packages

```text
github.com/k4k3ru-hub/onchain/go/venues/aerodrome
```

The `slipstream` package provides:

- canonical `protocol.PoolKey` values based on token pair and tick spacing
- Factory-based pool resolution and current Swap fee lookup
- exact-input and exact-output QuoterV2 calls
- `slot0`, active liquidity, and tick-spacing reads
- block-coherent local bid/ask quoting with dynamic fees and a bounded-age state cache
- block-range Swap filtering
- WebSocket Swap subscriptions

## Composition

Create one client for each deployment. Pool addresses must first be resolved from that deployment's Factory.

```go
import (
    "github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream"
    "github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/deployment"
    "github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
)

// Set token addresses and tick spacing for the configured pool.
var poolKey protocol.PoolKey
configuredDeployment, err := deployment.ByID(
	deployment.IDAerodromeBaseMainnetCurrent,
)
if err != nil {
	return err
}

client, err := slipstream.NewClient(slipstream.ClientParams{
	HTTPRPCClient: httpRPCClient,
	WSRPCClient:   wsRPCClient,
	Deployment:    configuredDeployment,
	SwapSources: []slipstream.SwapSource{
		{
			PoolAddress: poolAddress,
			PoolKey:     poolKey,
		},
	},
})
if err != nil {
	return err
}
```

All amounts use token base units. State and Quote methods accept an explicit block number; `nil` uses the latest state. Swap fees use a `1e-6` denominator and may change dynamically.

## Migration and compatibility

Migrated from `aerodrome/go/slipstream` at commit `08f197a1b9e9`.
The onchain module owns `slipstream`, `slipstream/protocol`, and
`slipstream/deployment` directly, with no aliases or dependency on the original
Aerodrome module. Existing users can continue using the original package;
named types from the two paths are distinct, so migrate related imports together.
The migration itself did not change quoting. Local state quoting was added separately on 2026-09-08.

Individual constructors remain available for independent HTTP and WS lifecycles:
`NewFactoryClient`, `NewQuoterClient`, `NewPoolStateClient`,
`NewSwapFilterClient`, and `NewSwapSubscriber`. The combined `NewClient` requires
both transports and composes all five groups through explicit dependencies.
Constructor composition and injected-RPC operation tests migrate with the code.

From `onchain/go`, run `GOWORK=off go test ./venues/aerodrome/...` and
`GOWORK=off go vet ./venues/aerodrome/...`.

## Local market-data quotes

`slipstream.NewStateCache(stateRPC, poolAddress, deployment.Factory, maxAge)`
constructs a separate cache per pool. `stateRPC` implements `slipstream.StateRPC`;
`*evm.HTTPClient` is one implementation. The supplied pool must belong to the
supplied factory. Keep the HTTP and WebSocket clients on the same chain.

Run `cache.Run(ctx, wsRPC)` under the application's subscription supervisor and
handle its returned error. It watches all logs from the pool and factory.
`cache.QuotePair(ctx, baseAmount, baseIsToken0)` returns the quote-token output
for selling the base amount and input for buying that same base amount, together
with `FeePPM`, `BlockNumber`, `BlockHash`, and the original `ObservedAt`.

All mutable reads, including `pool.fee()`, use one block number. A canonical hash
check follows any contract reads. Pool/factory logs invalidate the snapshot;
logs already covered by the verified snapshot block do not. During a quote,
notifications are also accepted if every intervening invalidation is a non-removed
log with the captured block number and hash. Mixed blocks, missing or conflicting
hashes, and lifecycle changes still reject the quote; a later matching log cannot
clear an earlier unsafe notification. Accepted quotes retain their observation time
and can be reused without another capture. Removed logs,
disconnects and reconnects invalidate it. Without an active subscription each
quote refreshes. Fee-module-only updates and missed notifications are bounded by
`maxAge`; reuse never advances `ObservedAt`. This is a bounded-age market-data
quote, not a promise that an old block's fee still applies at transaction execution.

The arithmetic handles initialized tick crossings using Slipstream's ten-word
tick layout and total liquidity net. Staked liquidity controls fee distribution,
not the trader's swap curve. Calls are bounded to 64 contract reads per pair and
2048 steps per direction; incomplete or invalid state returns an error.
Quoter methods remain available for execution checks and parity verification.
Custom fee modules whose output depends on transient swap execution state require
separate verification; the cache does not simulate arbitrary module logic.

### Cache diagnostics

`cache.Diagnostics()` returns a detached cumulative counter map. It does not log
or perform RPC. MarketHub emits deltas in its existing per-pool minute aggregate.

- `refresh.*`: one primary reason per refresh, in priority order: `no_snapshot`,
  `subscription_inactive`, `notifications_or_lifecycle`, `expired`. A rejected
  quote clears the snapshot, so its next refresh is `no_snapshot`.
- `notification.pool.*` / `notification.factory.*`: event counts relative to the
  **last accepted snapshot**, including ignored `same_block` notifications.
  Labels are `no_snapshot`, `newer_block`, `older_block`, `hash_mismatch`,
  `missing_hash`, `removed`. Removed/missing hashes take priority.
- `invalidation.*`: rejected quote counts relative to the **captured quote block**.
  `total` counts generation-change rejections; `newer_block`, `older_block`,
  `mixed_blocks`, `hash_mismatch`, `missing_hash`, `removed`, `lifecycle` may overlap.
  `hash_mismatch` identifies conflicting hashes when all observed events are in
  the captured block; mixed blocks are not treated as proven hash conflicts.
  `expired` counts age-only rejections separately.
- `lifecycle.connected` / `lifecycle.disconnected`: subscription transitions.
- `quote.concurrent_notifications_accepted`: quotes adopted despite intervening
  notifications verified to be in the captured block.

Counters are observational. Existing expiry, canonicality and invalidation rules
are unchanged. No event topics, addresses, endpoint URLs or payloads become labels.
