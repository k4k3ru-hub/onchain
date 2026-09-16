# Cetus CLMM

## LP APR for a specific pool

`api.NewClient(api.Config{})` composes an independent HTTP API group. Use
`client.Pools.Get(ctx, poolID)` with a `sui.Address` to read exactly one pool's
indexed yield statistics. The returned `*api.PoolStats` owns the v3 schema;
`Pools.List` and its v2 `Pool` return shape remain unchanged.
See the [compiled usage examples](api/example_test.go).

Get performs one read-only `POST /v3/sui/clmm/stats_pools` with
`{"pools":["<normalized pool ID>"],"display_all_pools":true}`. It validates the
provider code, total, result count and returned pool identity. An explicit empty
result returns `api.ErrPoolNotFound` (inspect with `errors.Is`); malformed,
ambiguous or unrelated results return errors. There is no list scan, retry or
fallback to another endpoint or pool.

- `PoolStats.TotalAPR` retains the provider's total decimal ratio.
- `Stats[]` retains `dateType` (`24H`, `7D`, `30D`), volume, fee and fee APR.
  MarketHub should select `24H` explicitly; missing 24h data stays unavailable.
- `MiningRewarders[]` explicitly associates each reward APR with `coinType`,
  decimals, display flag and `emissionsPerSecond`. In observed v3 responses,
  reward APR is a decimal ratio, unlike v2's percent-suffixed reward strings.
- `Raw` preserves the full pool JSON, including unknown farming metadata.
  `Vault` and `Extensions` also retain their original JSON. Missing numeric
  values remain nil; no calculation or conversion is performed.

On 2026-09-14, the official [Cetus application](https://app.cetus.zone/pools)
used `pools: [ID]` in its CLMM detail view. Its `useGetPoolList-BwXrpgs2.js`
and `path-BLIht4Yk.js` modules identified the v3 POST endpoint. Read-only probes
returned one matching result for each of two existing IDs, and `total: 0,
list: []` for `0x1`. The complete first probe is preserved in
`api/testdata/stats_pools_v3.json`. Tests use injected transports, not live APIs.
The v2 `pool_address` and `pools` query probes returned unfiltered lists; they
must not be treated as working selectors.

The v3 sample's total equals its 24h fee APR plus mining reward APR, but this is
not a guarantee of campaign eligibility, denominator, fee treatment or all
future totals. The v3 response establishes token attribution for its own mining
rewards; it does not establish a mapping for v2's anonymous reward slots.

## Indexed LP yield statistics

`venues/cetus/api` is a separate HTTP client for Cetus's mainnet
`GET /v2/sui/stats_pools`. `api.NewClient(api.Config{})` composes `Pools`;
`Config.HTTPClient` and `Config.BaseURL` allow transport injection. Construction
does not perform I/O. Each `Pools.List(ctx, api.ListParams{Limit: 20, Offset: 0})`
reads one page and returns the provider's `total` and `lp_list` as `PoolPage`.
Zero limit uses the provider default; offset is zero-based. Negative values are
rejected. The SDK does not automatically paginate, retry, cache or poll.
See the [compiled usage example](api/example_test.go).

`limit=1&offset=0` and `limit=1&offset=1` returned distinct pools in public
read-only probes on 2026-09-14. Ordering, server limits and concurrent-page
consistency are not guaranteed. This API exposes the provider's default pool
selection; it does not claim that every pool is included.

| Field | Preserved meaning |
| --- | --- |
| `Pool.TotalAPR` | Provider total as a decimal ratio: `0.39624…` means `39.624…%` |
| `Pool.APR.FeeAPR24h` | Provider 24h-based fee APR as a decimal ratio |
| `Pool.RewarderAPR` | Unmodified percent-suffixed strings, including all reward slots |
| `Pool.Fee24h`, `VolumeInUSD24h`, `PureTVLInUSD` | Decimal text without float64 conversion |
| `Object.RewarderManager`, `StableFarming` | Raw JSON preserving source metadata whose mapping/schema is not fully verified |

Numeric pointers distinguish absent/null from explicit zero. Reward slots can
also be null. Missing APR is never calculated from another field. Total APR is
returned as supplied even if it differs from the components; reward slots are
not paired with the embedded reward manager by index. Closed/paused flags,
coin identities, vault references and reward display flags remain available.
The embedded object is not a full `clmm.ParsePoolRewards` input.

The agreed MarketHub reference is a 24h-based fee APR. MarketHub will normalize
units and preserve the reward calculation basis separately; the reported total
does not establish that all its components use a 24h observation window.
The SDK does not determine eligibility, gross/net protocol-fee treatment,
position returns or a cross-venue denominator. It never substitutes 7d/30d APR.

Failures include non-2xx HTTP responses (`*api.HTTPError`, retaining `RetryAfter`),
nonzero provider codes (`*api.APIError`), malformed/missing envelopes, invalid pool
addresses and malformed numeric metrics. Cancellation and transport/decoder
errors remain inspectable through `errors.Is`/`errors.As`; error messages omit
HTTP bodies and upstream application messages. Responses are limited to 16 MiB.
The default HTTP client timeout is 15 seconds; callers can supply a bounded context.

Tests use injected transports. `api/testdata/stats_pools.json` retains the first
pool and envelope from the public 2026-09-13 capture, including the original
reported total; it is a reduced fixture, not a complete page or current snapshot.
Tests cover composition, query encoding, source precision, null/zero distinctions,
reward metadata, errors, cancellation and response limits.

Sources: [official mainnet configuration](https://github.com/CetusProtocol/cetus-sdk-v2/blob/main/packages/clmm/src/config/mainnet.ts),
[official statistics consumer](https://github.com/CetusProtocol/cetus-sdk-v2/blob/main/packages/clmm/src/modules/poolModule.ts),
and [APR research](../YIELD_SUI_RESEARCH_20260913.md).

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
