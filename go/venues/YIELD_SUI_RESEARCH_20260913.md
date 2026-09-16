# Sui LP yield acquisition and APR semantics

Research date: 2026-09-13. Scope: Cetus CLMM, Bluefin Spot CLMM, Turbos CLMM,
and Momentum CLMM. The user authorized work on the onchain main branch.
This document records source inspection and public read-only HTTP observations;
it does not introduce RPCs, adapters, trading operations, or a release.

## Agreed ownership

- MarketHub.Yield covers `lp`, `lending`, and `staking`; Funding carry remains
  with MarketHub.Carry. Venue category is independently `cex` or `dex`.
- onchain returns provider-owned data, preserving its units, missing values,
  periods, identities, and reward metadata. It does not silently manufacture APR.
- MarketHub owns scheduling, freshness, normalization, and aggregation.
- Pool yield statistics and wallet/price-range-specific position returns are
  different outputs. The former does not establish the latter.

## Acquisition results

| Venue | Successful public GET | Observed envelope / sample | Existing Go SDK |
| --- | --- | --- | --- |
| Cetus | `https://api-sui.cetus.zone/v2/sui/stats_pools` | `code`, `msg`, `data.total`, `data.lp_list`; default page returned 20 pools, total 44,019 | Raw `clmm.PoolRewards` only; indexed statistics adapter absent |
| Bluefin | `https://swap.api.sui-prod.bluefin.io/api/v1/pools/info?page=1&limit=1` | Array containing one pool | Statistics adapter absent |
| Turbos | `https://api.turbos.finance/pools/v2?poolId=0x5eb2dfcdd1b15d2021328258f6d5ec081e9a0cdcfa9e13a0eaeb9b5f7505ca78` | One pool object | Statistics adapter absent |
| Momentum | `https://api.mmt.finance/pools/v3` | `status`, `message`, `data`; 89 pools, all with `aprBreakdown` | `api.Pools.List/Get/RewardsAPY` already implemented |

These are bounded samples, not complete coverage or availability guarantees.
The first three venue responses from the preceding investigation and the new
Cetus response were inspected together; their timestamps are not synchronized.
No credentials, wallets, or transaction submissions were used.

Source and API differences:

- Cetus's older documented `/v2/sui/swap/count` returned HTTP 404. The official
  SDK v2 mainnet configuration selects `stats_pools`; its pool module reads
  `data.lp_list`. Do not implement the older envelope from documentation alone.
- Turbos's short list query returned HTTP 400, `poolId is required`; the older
  APR documentation's example pool ID returned 404 when specified explicitly.
  The repository test's pool ID above returned 200. This does not establish
  support for pagination or the documentation's complete list query.
- That Turbos response identifies SUI/wUSDC, including the Wormhole coin type.
  The existing MarketHub test labels that pool SUI/USDC with a different coin
  type. Do not use the fixture label as proof of the live asset identity. This
  research did not change the fixture or inspect production configuration.

## Total APR and units

All four successful samples contain a provider total and fee/reward components.
Totals are optional provider observations, not universally mandatory fields.

| Venue | Total | Fee component | Reward component | Units |
| --- | --- | --- | --- | --- |
| Cetus | `total_apr` | `apr.fee_apr_24h` | `rewarder_apr[]` | Total/fee are decimal ratios; rewards are percent-suffixed strings in the observed schema |
| Bluefin | `day/week/month.apr.total` | Same period's `apr.feeApr` | Same period's `apr.rewardApr` | Percentage-valued decimal strings; supported by sample arithmetic, exact backend formula unverified |
| Turbos | `apr`, `apr_7d` | `fee_apr`, `fee_7d_apr` | `reward_apr`, `reward_7d_apr` | Percentage-valued JSON numbers; official formula multiplies by 100 |
| Momentum | `aprBreakdown.total` | `aprBreakdown.fee` | `aprBreakdown.rewards[].apr` | Percentage-valued decimal strings; official SDK formula uses percent units |

Cetus provides an especially important mixed-unit example:

```text
total_apr       = 0.3962404734165865
fee_apr_24h     = 0.2406720960157855
rewarder_apr    = [0.41760996615576%, 15.13922777392434%, 0%, 0%, 0%]

39.62404734165865% = 24.06720960157855% + 0.41760996615576% + 15.13922777392434%
```

Decimal arithmetic reproduced this relationship for all 20 sampled Cetus pools.
It is strong empirical evidence for these fields' units, not a published
guarantee about every future schema version. Preserve raw fields in onchain.

For all 89 Momentum pools, the maximum absolute difference between the supplied
total and fee plus all reward APRs was `0.000000000000000500` percentage points.
For the Bluefin sample, all three period totals equal their respective fee plus
reward component exactly as decimal strings. Turbos's sample has zero rewards,
and total equals fee APR for both observed periods; positive-reward arithmetic
was not exercised.

Bluefin's top-level `totalApr` was `"0"` while `day.apr.total` was
`"59.9658571602257851"`. Preserve both fields if exposed by the adapter; use an
explicitly selected period for the MarketHub reference metric. The reason for
the top-level discrepancy remains unknown. It must not replace the period total.

## Periods and denominators

| Venue | Evidence for period | Denominator / calculation limits |
| --- | --- | --- |
| Cetus | Current response exposes 24h fee APR, `fee_24_h`, `vol_in_usd_24h`; older docs describe 24h/7d/30d fields absent from this sample | `pure_tvl_in_usd` is available, but fee APR does not exactly equal `fee_24_h * 365 / pure_tvl_in_usd`; backend denominator and update alignment remain unverified |
| Bluefin | `day`, `week`, `month` each contain fees, volume and APR components | Fee APR is close to percent annualization of period fees/current TVL, but not exact; exact rolling boundaries, average/current TVL, reward averaging and protocol-fee treatment are unverified |
| Turbos | Official docs define a 24h baseline; sample also has explicitly 7d-labelled APR/fees | Sample reproduces `fee_24h_usd * 365 / liquidity_usd * 100` and `fee_7d_usd * 365 / 7 / liquidity_usd * 100`; no 30d APR field despite 30d volume |
| Momentum | 24h fees/volume; SDK annualizes with 365 days | SDK has both whole-pool TVL and range/liquidity-based paths; do not assume the indexed API uses one universal TVL denominator |

Momentum's `calculatePoolAPR` computes fee percent APR from 24h fees/current TVL
and reward percent APR from daily reward value/current TVL. Reward emissions are
converted from Q64 base-unit flow rates using token decimals. It skips ended
rewarders. However, `getRewardsAPY` also contains a non-stable-pool path using a
roughly +/-5% price range and `liquidityHM` (with a fallback calculation).
Current `getPool` reads the indexed API; finding SDK math does not prove which
backend calculation produced a particular response.

In the Momentum sample, supplied fee APR divided by simple 24h-fees/current-TVL
percent annualization ranges from approximately 0.15256 to 7.62099 over pools
with positive fees and TVL. Neither the cause nor the server's exact calculation
can be established from that ratio alone. The earlier simplified description
of all Momentum APR as fees/TVL annualization is therefore insufficient.

No APR endpoint proves automatic compounding. Preserve Momentum's legacy `apy`
and `RewardsAPY` labels rather than mechanically converting or relabelling them.
The official SDK deprecates `apy` in favor of `aprBreakdown.total`, but that alone
does not establish a general APY-to-APR conversion rule.

## Reward conditions and attribution

| Venue | Confirmed source behavior / conditions | What the adapter must retain or avoid assuming |
| --- | --- | --- |
| Cetus | Official CLMM docs describe in-range fee earning and fee-contribution-based mining. Separate Farms SDK stakes position NFTs into farm pools and includes effective-range checks | Preserve closed/paused flags, reward slots, display flags, embedded manager, `stable_farming` and vault references when available. Do not merge farm rewards without identifying eligibility |
| Bluefin | Official guide ties fees/rewards to the selected price range. API sample supplies reward token, daily amounts/USD values, per-second emissions and end time | Preserve end time, paused flag, token identity/decimals and period-labelled reward APR. Do not extend a finite campaign indefinitely or infer universal eligibility from pool membership |
| Turbos | Official position APR implementation returns zero when outside the selected range and calculates rewards from Q64 emissions and token prices | Preserve `reward_infos`, emission rates, vault/coin identity, update time, unlocked/locked/farm/vault fields. Sample rewards were all zero; funding and positive-reward eligibility were not verified live |
| Momentum | Official mining docs describe active fee contribution, position NFTs and optional extra incentives for staked NFTs. SDK filters `hasEnded` rewarders | Preserve coin type, `amountPerDay`, flow rate, `hasEnded`, farm ID/source, deprecated flag and source timestamp. Pool reference APR is not a wallet eligibility decision |

Cetus's sample always has five `rewarder_apr` entries but its embedded onchain
reward manager has zero to three rewarders. Index-to-token correspondence is not
confirmed. Do not zip/truncate these arrays or assign token symbols by position
without a verified mapping. All 20 sampled `stable_farming` fields were null, so
that sample does not establish the response shape or APR inclusion of active farms.

For protocol fees, preserve source rates but do not subtract a second fee from
the reported APR. Cetus and Momentum documentation discuss protocol shares;
that does not prove whether each indexed APR is gross or already net of that
share. Turbos's sample formula matches the supplied fee field, but this alone
does not identify how much of that field is paid to LPs.

## Implementation-ready acquisition boundary

Reuse Momentum's separate `api` composition boundary for new Cetus, Bluefin and
Turbos HTTP clients. Keep CLMM chain readers and these indexed API clients
independent and inject HTTP transports. Preserve decimal text and missing/null
values; numeric JSON APRs must not first pass through float64. Keep pool addresses,
coin types, source periods, source timestamps and reward metadata in owning
venue packages. Statistics responses are not necessarily full Sui object-reader
payloads: Cetus's embedded object is not automatically valid input to the existing
`ParsePoolRewards` contract.

No automatic migration to another pool, hidden APR fallback, arbitrary fee/reward
sum, or provider retry loop belongs in these reads. Application policy will select
periods and normalize units. Unknown denominator/eligibility stays unknown.

Remaining work before normalized cross-venue comparison:

1. Define and verify Cetus reward-slot attribution and the active-farm response.
2. Confirm indexed APR gross/net basis and exact denominator/window boundaries
   where the public source does not specify them. Preserve provider-reported APR
   without claiming apples-to-apples normalization in the interim.
3. Verify pagination, per-pool selection and error envelopes for each new adapter;
   test missing metrics, unrelated pool responses and conflicting identities.
4. Cover positive Turbos rewards and representative paused/ended/deprecated cases.
5. Agree MarketHub period-selection and unavailable-metric policy separately from
   provider acquisition. This research does not fix those public RPC parameters.

## Evidence and verification

Read-only `curl` requests were required after sandbox DNS failures; requests
succeeded with approved network access. Downloaded official SDK archives and
public responses were inspected under `/private/tmp`. Python Decimal checks
used the captured responses, not new asynchronous calls for each metric.
These checks verify observed arithmetic, not upstream correctness or execution
profitability. No Go tests/builds were run because only documentation changed.

Source response locations from this session (temporary, not permanent archives):

- `/private/tmp/k4k3ru-yield-cetus-stats.json`
- `/private/tmp/k4k3ru-yield-bluefin.json`
- `/private/tmp/k4k3ru-yield-turbos-configured.json`
- `/private/tmp/k4k3ru-yield-momentum.json`

Official sources inspected on 2026-09-13:

- [Cetus older APR documentation](https://cetus-1.gitbook.io/cetus-developer-docs/developer/via-sdk/features-available/apr-correlation-calculation)
- [Cetus current mainnet configuration](https://github.com/CetusProtocol/cetus-sdk-v2/blob/main/packages/clmm/src/config/mainnet.ts)
  and [statistics consumer](https://github.com/CetusProtocol/cetus-sdk-v2/blob/main/packages/clmm/src/modules/poolModule.ts)
- [Cetus fees](https://cetus-1.gitbook.io/cetus-docs/clmm/fees)
  and [liquidity mining](https://cetus-1.gitbook.io/cetus-docs/clmm/liquidity-mining)
- [Cetus Farms SDK](https://github.com/CetusProtocol/cetus-sdk-v2/tree/main/packages/farms)
- [Bluefin pool API](https://bluefin-exchange.readme.io/v2.0.1/reference/spot-api-getpoolsinfo)
  and [LP range conditions](https://learn.bluefin.io/bluefin/bluefin-spot-clmm/tutorials/adding-liquidity-creating-a-position)
- [Turbos APR documentation](https://turbos.gitbook.io/turbos/developer-docs/via-sdk/clmm/apr-calculation)
  and [position APR implementation](https://github.com/turbos-finance/turbos-clmm-sdk/blob/main/src/lib/nft.ts)
- [Momentum pool math and operations](https://github.com/mmt-finance/clmm-sdk/blob/main/src/modules/poolModule.ts)
  and [types](https://github.com/mmt-finance/clmm-sdk/blob/main/src/types.ts)
- [Momentum fees](https://docs.mmt.finance/core-products/momentum-dex/core-mechanics/fees)
  and [liquidity mining](https://docs.mmt.finance/core-products/momentum-dex/core-mechanics/liquidity-mining)
