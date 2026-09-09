# Pool yield acquisition

Research and initial SDK adapters, 2026-09-09.

SDK operations perform one read and return source data. They do not schedule
polling, retry, publish snapshots, run quotes, calculate position APR, or normalize
different providers' annualization assumptions. The application owns those tasks.
An absent metric is unavailable, not zero. APY is not relabelled APR. Receiving a
response is not evidence that its indexed data is current.

## Sui

| Venue | Verified source | Implementation status |
| --- | --- | --- |
| Momentum | Official SDK `/pools/v3`, `/pools/v3/{id}`, `/pools/v3/rewarders-apy/{id}` | `momentum/api`: pool statistics, APR breakdown, legacy APY and reward configuration |
| Cetus | Pool `rewarder_manager` in official SDK object decoder | `cetus/clmm.PoolRewards` and `ParsePoolRewards`: raw reward state; no APR calculation |
| Turbos | Official APR documentation identifies indexed pool statistics | HTTP response schema unverified; no new API implementation |
| Bluefin | Official Spot `GET /api/v1/pools/info` and pool stats documentation | Yield response schema unverified; no new API implementation |

HTTP probes to Momentum, Turbos and Bluefin returned 403 in the research
environment. Momentum types and envelopes were verified against its official
TypeScript SDK and pool fixture; operation tests use injected transports, not a
successful live API response. Turbos and Bluefin remain pending schema verification.
Cetus tests cover flattened GraphQL and nested JSON-RPC object representations;
they are not live reward-APR parity tests.

Sources:

- [Momentum types](https://github.com/mmt-finance/clmm-sdk/blob/main/src/types.ts),
  [operations](https://github.com/mmt-finance/clmm-sdk/blob/main/src/utils/poolUtils.ts),
  [official pool fixture](https://github.com/mmt-finance/clmm-sdk/blob/main/tests/__test_data__/all-pools.json).
- [Cetus pool decoder](https://github.com/CetusProtocol/cetus-clmm-sui-sdk/blob/main/src/utils/common.ts).
- [Turbos APR documentation](https://turbos.gitbook.io/turbos/developer-docs/via-sdk/clmm/apr-calculation).
- [Bluefin pool API](https://bluefin-exchange.readme.io/v2.0.1/reference/spot-api-getpoolsinfo).

## EVM: researched, not implemented

Uniswap v3/v4 subgraphs expose indexed pool statistics suitable as inputs for
reference fee yield. A deployment-specific endpoint and supported schema are
needed; do not hard-code one endpoint for all networks. Rewards require a separate
incentive source and cannot be inferred from a pool's swap fee.

Aerodrome yield needs staking eligibility and its denominator/measurement window
preserved. Fees and emissions must not automatically be summed when they apply
to different staking conditions.

- [Uniswap v3 entities](https://developers.uniswap.org/docs/ecosystem/subgraphs/concepts/v3/entities).
- [Uniswap v4 schema](https://github.com/Uniswap/v4-subgraph/blob/main/schema.graphql).
- [Uniswap v3 liquidity mining](https://developers.uniswap.org/docs/protocols/v3/concepts/liquidity-mining).
- [Aerodrome liquidity documentation](https://github.com/aerodrome-finance/docs/blob/main/content/liquidity.mdx).

## Solana

Raydium's v3 API exposes day/week/month pool statistics with separate fee APR and
reward APR entries. Its official SDK defines the response types and reward token
metadata. `solana/raydium/api` now provides `Pools.Get` with injected-transport
operation tests. A live API success has not been verified in this environment.

Meteora documents an indexed DLMM pool API; its current response schema and
reward attribution need verification before adding typed operations.

- [Raydium API types](https://github.com/raydium-io/raydium-sdk-V2/blob/master/src/api/type.ts),
  [operations](https://github.com/raydium-io/raydium-sdk-V2/blob/master/src/api/api.ts).
- [Meteora DLMM Data API](https://github.com/MeteoraAg/docs/blob/main/developer-guides/dlmm/api-reference/overview.mdx).

## Application integration still outstanding

VenueClient will acquire yield independently of Trade-triggered quotes and replace
an Aggregator-owned `AMMPoolYieldSnapshot`. Pool identity, source, observation time,
source time, window, units, denominator, reward token and availability must survive
normalization. MarketHub scheduling, the shared yield snapshot, and
`MarketHub.Yield.Get/Subscribe` are not implemented by this SDK change.
