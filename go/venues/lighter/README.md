# Lighter Go SDK

Public market data for the Robinhood Chain Lighter instance. REST and WebSocket
have separate composition roots and lifecycles. No API key is required.
Trading, signing, account APIs, deposits, withdrawals, and K4K3RU routing integration
are outside this implementation.

## REST

```go
import (
    "context"

    "github.com/k4k3ru-hub/onchain/go/venues/lighter/rest"
    "github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets/order_books"
)

func listMarkets(ctx context.Context) (*order_books.Result, error) {
    client, err := rest.NewClient(rest.ClientParams{})
    if err != nil {
        return nil, err
    }
    return client.Markets.OrderBooks.Send(ctx, order_books.Params{})
}
```

| Client operation | Endpoint | Parameters |
| --- | --- | --- |
| `Markets.OrderBooks.Send` | `/api/v1/orderBooks` | Optional `MarketID` pointer, `Filter` (`all`, `spot`, `perp`) |
| `Markets.OrderBookDetails.Send` | `/api/v1/orderBookDetails` | Same filters; separate perp and spot result arrays |
| `Markets.OrderBookOrders.Send` | `/api/v1/orderBookOrders` | `MarketID`, `Limit` (1–250) |
| `Markets.RecentTrades.Send` | `/api/v1/recentTrades` | `MarketID`, `Limit` (1–100) |
| `Markets.Fundings.Send` | `/api/v1/fundings` | `MarketID`, `Resolution` (`1h` or `1d`), `StartTimestamp`, `EndTimestamp`, `CountBack` |
| `Markets.FundingRates.Send` | `/api/v1/funding-rates` | Empty parameters; cross-venue funding comparison |

Import `Params` and `Result` from the operation's owning package, as in the example.
Market ID zero is valid. Discover IDs from this instance's metadata; do not reuse
IDs from another instance. `OrderBookOrders` returns individual orders, whereas
WebSocket order books contain price levels. Funding history uses the upstream
timestamp units without conversion; `CountBack: 0` requests all available points
within the server's limit (documented maximum 750).

Defaults: `https://api.rh.lighter.xyz`, 10-second request timeout, 8 MiB response
limit. `ClientParams` accepts an `HTTPClient`, base URL and limits. Non-success
HTTP status or API code produces an inspectable `*rest.ResponseError`. Errors
omit remote bodies/messages; underlying transport errors remain inspectable via
`errors.Is` / `errors.As`. The default HTTP client does not follow redirects.

## WebSocket

```go
import (
    "context"
    "errors"

    "github.com/k4k3ru-hub/onchain/go/venues/lighter/websocket"
    "github.com/k4k3ru-hub/onchain/go/venues/lighter/websocket/protocol"
    "github.com/k4k3ru-hub/onchain/go/venues/lighter/websocket/subscriptions"
)

func watch(ctx context.Context, marketID int64, consume func(*protocol.Message) error) (err error) {
    client, err := websocket.NewClient(websocket.ClientParams{ReadOnly: true})
    if err != nil {
        return err
    }
    if err := client.Connect(ctx); err != nil {
        return err
    }
    defer func() { err = errors.Join(err, client.Close()) }()
    if err := client.OrderBook.Subscribe(ctx, subscriptions.Params{MarketID: marketID}); err != nil {
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

Groups: `OrderBook`, `BBO`, `Trades`, `MarketStats`, `SpotMarketStats`.
Each supports `Subscribe` and `Unsubscribe`. Only the two statistics groups
support `subscriptions.Params{All: true}`. Subscribe success confirms sending,
not server acceptance; read acknowledgements and errors with `Recv`.

The constructor performs no network I/O. `Connect` starts a session with 30-second
control pings. Defaults: `wss://api.rh.lighter.xyz/stream`, 10-second connect timeout,
5-second write timeout, 8 MiB message limit. Keep receiving to process server control
frames; application `ping` messages receive an automatic `pong` during `Recv`.
Inject a SDK-owned `Dialer` / `Connection` to substitute the transport.

`Connect`'s context bounds dialing only. `Close` ends the session and waits for
keepalive to finish; it also reports a retained terminal session error. Read/write
cancellation can close the socket. Call `Close` before reconnecting, then explicitly
resubscribe. There is no automatic reconnect or order-book reconstruction.

`subscribed/order_book` contains the initial snapshot; `update/order_book` contains
changes (size zero deletes a level). Verify each delta's `BeginNonce` against the
previous `Nonce`. On a gap, resubscribe for a fresh snapshot. `Offset` is not a
contiguous sequence. Single-market and all-market statistics decode into maps keyed
by market ID. Regular and liquidation trades remain separate. Unknown message types
retain type/channel, but unknown payload fields are not retained.

## Data and architecture

Decimal strings remain strings; numeric decimal fields use `json.Number` to avoid
float64 rounding. Amounts, timestamps, and rates retain upstream units. In market
statistics, `CurrentFundingRate` is an estimate and `FundingRate` is the settled rate
at `FundingTimestamp`; `/funding-rates` is a different cross-venue dataset.

`rest.NewClient` composes `markets.NewClient`, which composes all six operations
around an explicit executor. `websocket.NewClient` composes subscription groups
around the session sender. Operation constructors accept small interfaces for
isolated tests. Shared REST models live in `lighter/protocol`; WebSocket messages
live in `lighter/websocket/protocol`. There are no root aliases or DI containers.
The socket adapter uses the module's existing `gorilla/websocket` dependency.

Robinhood's Lighter deployment has separate contracts, sequencer and liquidity from
Lighter Core. Endpoint overrides are available for compatible deployments, but this
SDK's documented contract targets the Robinhood instance.

Sources: [Robinhood Lighter domains](https://docs.robinhood.com/chain/lighter-domains/),
[API reference](https://apidocs.rh.lighter.xyz/llms.txt),
[WebSocket reference](https://apidocs.rh.lighter.xyz/docs/websocket-reference).

## Verification

From `onchain/go`:

```sh
go test -race ./venues/lighter/...
go vet ./venues/lighter/...
```

Tests use injected HTTP/executor/dialer fakes and a local WebSocket server; no live
service or credentials are required.
