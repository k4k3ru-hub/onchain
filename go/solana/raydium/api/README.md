# Raydium indexed pool statistics

Compose `github.com/k4k3ru-hub/onchain/go/solana/raydium/api.NewClient` with
`api.Config{HTTPClient: httpClient}`. `client.Pools.Get(ctx, poolAddress)` performs
one `/pools/info/ids` request and returns exactly the requested pool.

The client preserves day/week/month statistics, separate fee/reward APR values,
TVL, and default reward metadata from the official API. Numeric values retain
decimal text in `*json.Number`; missing values remain nil. It does not infer reward
eligibility or sum reward rates, convert units, poll, retry, or run local quotes.

HTTP errors expose `*api.HTTPError` with status and Retry-After. An omitted HTTP
client uses a 15-second timeout. Context cancellation is preserved in the error
chain; response error bodies are not included in errors. An injected HTTP client
owns transport policy, including redirect behavior and authentication.

Source: [official API operations](https://github.com/raydium-io/raydium-sdk-V2/blob/master/src/api/api.ts)
and [response types](https://github.com/raydium-io/raydium-sdk-V2/blob/master/src/api/type.ts).
Tests use injected responses derived from those types; live APR parity is unverified.
