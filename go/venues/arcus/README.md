# Arcus Perps Go SDK

Public perpetuals market data for Arcus on Robinhood Chain. REST and WebSocket
have separate composition roots with manually injected transports. No credentials
are required. The Spot RFQ API, trading/signing, account data and K4K3RU routing
integration are outside this SDK's scope.

## REST

```go
import (
    "context"

    "github.com/k4k3ru-hub/onchain/go/venues/arcus/rest"
    "github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets/list"
)

func listMarkets(ctx context.Context) (*list.Result, error) {
    client, err := rest.NewClient(rest.ClientParams{})
    if err != nil {
        return nil, err
    }
    return client.Markets.List.Send(ctx, list.Params{})
}
```

Import request and result types from each operation's owning package.

| Operation | Endpoint | Parameters |
| --- | --- | --- |
| `Markets.List.Send` | `GET /v1/markets` | Optional `Market` |
| `Markets.OrderBook.Send` | `GET /v1/l2OrderBook/{market}` | `Market`, optional `NLevels`, `SigFigs`, `RoundStep` |
| `Markets.BBO.Send` | `GET /v1/bbo/{market}` | `Market` |
| `Markets.Trades.Send` | `GET /v1/trades` | `Market`, optional `Limit`, `From`, `To` |
| `Markets.Candles.Send` | `GET /v1/candles` | `Market`, `Timeframe`, `To`, optional `From` or `CountBack` |
| `Markets.FundingRates.Send` | `GET /v1/fundingRates` | `Market`, optional `Limit`, `From`, `To` |

Discover display names and IDs with `Markets.List`. Use display names such as
`BTC-USD` for WebSocket subscriptions. `Market: "0"` is an explicit numeric REST
selection; an empty `list.Params.Market` lists all markets.

Optional numeric zero values are omitted: server defaults are 20 book levels and
1000 history rows. The SDK rejects values above the documented maxima (100 levels,
1000 history rows, 1500 candles), even where the server would clamp them. Optional
`From` / `To` pointers distinguish omission from invalid zero timestamps.
`SigFigs` accepts 2–5 and `RoundStep` accepts 1, 2, 5; the server only applies
`RoundStep` when `SigFigs` is 5. Market parameters must be ASCII letters, digits,
hyphens or underscores, at most 128 bytes, for unambiguous URL path segments.

Candle timeframes: `1m`, `3m`, `5m`, `15m`, `30m`, `1h`, `2h`, `4h`, `8h`, `12h`,
`1d`, `3d`, `1w`. `From` and `CountBack` are mutually exclusive. Candle windows
are `[from, to)`; trade and funding history bounds are inclusive. Trade page
boundaries can overlap; deduplicate by `TradeID`. Pagination is explicit.

Defaults: `https://api.arcus.xyz`, 10-second request timeout and 8 MiB response
limit. Set `BaseURL: rest.RobinhoodTestnetURL` for testnet. The base URL excludes
`/v1`. An injected `HTTPClient` must honor request contexts. The default client
does not follow redirects; retry/rate-limit policy is left to the caller.
Non-success HTTP statuses return an inspectable `*rest.ResponseError`.

## WebSocket

```go
import (
    "context"
    "errors"

    "github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket"
    "github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket/protocol"
    "github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket/subscriptions"
)

func watch(ctx context.Context, market string, consume func(*protocol.Message) error) (err error) {
    client, err := websocket.NewClient(websocket.ClientParams{})
    if err != nil {
        return err
    }
    if err := client.Connect(ctx); err != nil {
        return err
    }
    defer func() { err = errors.Join(err, client.Close()) }()
    if err := client.OrderBookUpdates.Subscribe(ctx, subscriptions.Params{
        Market: market,
        NLevels: 20,
    }); err != nil {
        return err
    }
    for {
        message, err := client.Recv(ctx)
        if err != nil {
            return err
        }
        if err := consume(message); err != nil {
            return err
        }
    }
}
```

| Group | Channel | Data |
| --- | --- | --- |
| `OrderBook` | `l2Orderbook` | Periodic snapshots |
| `OrderBookUpdates` | `l2OrderbookUpdates` | Initial snapshot then incremental updates |
| `BBO` | `bbo` | Nullable best bid/ask |
| `Trades` | `trades` | Array of fills from one taker match; no initial trade snapshot |
| `Markets` | `markets` | Full snapshots keyed by market ID; subscribe with empty params |
| `OraclePrices` | `oraclePrices` | Oracle and mark prices |
| `PredictedFunding` | `predictedFunding` | Next-hour estimate; initial contents may be empty |

All groups expose `Subscribe` and `Unsubscribe`. The subscription ID is the
market display name, except on the global `markets` channel, which has no ID.
Only book groups accept depth/aggregation options. Unsubscribe echoes `SigFigs`
and `RoundStep` to identify the same aggregation view. Optional `Snapshot: &false`
suppresses the initial snapshot on supported delta channels; the server ignores
it for snapshot-only channels. Normal usage should retain the default snapshot.

Send success does not confirm server acceptance. `Recv` returns `subscribed`,
`channel_data`, `unsubscribed` and `degraded` notices. A `subscribed` message can
already contain the initial snapshot. `degraded` means the server has retained the
subscription and will retry the unavailable snapshot: check `Reason` and
`RetryAfterMS` and do not treat the book as initialized. Remote error responses
return `*websocket/protocol.ResponseError`. Unknown channels retain raw `Contents`.

The SDK does not reconstruct a book. Level tuples are `[price, size]` strings;
updates set absolute sizes and `"0"` deletes a level. Track `LastSequenceID` per
market and aggregation view. Discard deltas already covered by the snapshot and
resubscribe for a fresh snapshot on a sequence gap. `GlobalSequenceID` orders
cross-market events and is a separate sequence. Reconnect requires a fresh
snapshot and explicit resubscription; no automatic reconnect occurs.

Defaults: `wss://api.arcus.xyz/v1/ws`, 10-second connection timeout, 5-second
write timeout, 30-second control pings and 8 MiB message limit. Use
`websocket.RobinhoodTestnetURL` to select testnet. Constructors do not dial.
`Connect`'s context bounds dialing only; `Close` owns the session lifetime and
waits for keepalive to end. Use one receive loop so server control frames are
processed. Read/write cancellation may close the socket. `Close` also reports
retained terminal errors; call it before reconnecting.

## Precision and units

Shared models live in `arcus/protocol`, streaming envelopes in
`arcus/websocket/protocol`. Decimal prices, sizes and rates remain strings.
Sequence IDs are `uint64`; integer epochs retain full precision. Nullable market
state fields use pointers, so unknown state is distinct from false or zero.

| Fields | Units |
| --- | --- |
| REST history `From`, `To`; trade timestamp; candle open time; funding history time; book/BBO timestamp | Unix microseconds |
| Oracle `Epoch`, `MarkEpochNanos` | Unix nanoseconds |
| Market `NextFundingAt`, listing/session expansion epochs | Unix seconds |
| Funding history `FundingRate`, prediction `Rate1h` | Hourly decimal rate |

Use `time.Now().UnixMicro()` for REST candle bounds. Seconds/milliseconds supplied
as history bounds are rejected. Zero mark price means unavailable; the SDK does
not substitute the oracle price. Realized and predicted funding remain distinct.

## Composition and verification

`rest.NewClient` composes `markets.NewClient` and all six operation clients around
one executor. Each operation supports an injected `transport.Executor`.
`websocket.NewClient` composes seven channel clients around a session sender;
`Dialer` and `Connection` are SDK-owned interfaces. The default socket adapter uses
the module's existing Gorilla dependency. No new production dependency is added.

```sh
# From onchain/go
go test -race ./venues/arcus/...
go vet ./venues/arcus/...
```

Tests use injected executors/transports and a local WebSocket server. They require
no credentials or live service. README examples are compiled by the Go test suite.

Specification sources (checked 2026-09-06):
[REST introduction](https://docs.arcus.xyz/api-reference/introduction),
[endpoint index](https://docs.arcus.xyz/llms.txt),
[WebSocket overview](https://docs.arcus.xyz/api-reference/websocket),
[AsyncAPI](https://docs.arcus.xyz/api-reference/asyncapi.yaml),
[mainnet changelog](https://docs.arcus.xyz/changelog).
Mainnet and testnet features can differ during rollout; optional fields or
uncached forecasts may be absent.
