# Bluefin pool statistics

`api.NewClient(api.Config{HTTPClient: client})` composes `Pools` without
network I/O. Call `client.Pools.List(ctx, page, limit)` with a one-based page and
limit 1–500. The default endpoint is
https://swap.api.sui-prod.bluefin.io/api/v1/pools/info.

Values retain provider units and precision. Period APR and feeRate are percentages,
not ratios; callers must normalize explicitly. Missing/null fields stay nil.
The misleading legacy top-level totalApr is intentionally not mapped.
Rewards are aggregate campaign metadata, not per-token APR calculations.

One call fetches one page with no retry or background work. Consumers own timeouts,
pagination limits, cache freshness and Retry-After handling. HTTP errors retain
status and Retry-After without response bodies. Default HTTP timeout: 15 seconds.
