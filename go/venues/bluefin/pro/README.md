# Bluefin Pro perpetual protocol

`github.com/k4k3ru-hub/onchain/go/venues/bluefin/pro` owns the Pro REST models,
Sui PersonalMessage bytes, BCS order hash and stream subscriptions. Spot swaps
remain in the sibling `../spot` package; `../api` provides Spot pool statistics.

The implementation and tests were moved from the local `bluefin/go/pro` package
on 2026-10-19 without changing protocol behavior. The original module is retained,
but applications must use this owning package consistently: Go types from the two
import paths are distinct. No aliases or dependency on the standalone Bluefin
module are introduced.

```go
import "github.com/k4k3ru-hub/onchain/go/venues/bluefin/pro"

client, err := pro.NewClient(pro.Config{Network: pro.Testnet})
if err != nil {
    return err
}
info, err := client.Exchange().Info(ctx)
if err != nil {
    return err
}
market, collateral, err := info.ResolveSUI(pro.Testnet)
```

`NewClient` composes `Exchange`, `Auth`, `Account` and `Trade` without network I/O. Inject `Config.Transport` for tests or application transport policy. Redirects are disabled, private responses are excluded from errors, and order creation is never retried.

`NewStreamClient(network, dialer)` has a separate lifetime. Supply a `Dialer` implementing the package's `Connection` interface, including ping/pong and context cancellation. `Market` and `Account` return after a successful subscription acknowledgment. One consumer owns each subscription's `Next` calls; `Close` is idempotent.

Signers implement `SignPersonalMessage([]byte) ([]byte, error)`. `SignMessage` returns the Base64 Sui serialized signature. No private key is held by the REST or stream client. Login JSON is compact; order JSON follows Bluefin's two-space indentation. The order hash follows the official BCS representation. Tests include an independent Sui TypeScript signature and BCS hash fixture.

The initial market validation targets active SUI/USDC. Quantities, prices and fees use E9 strings. `DecimalToE9` and `FormatE9` preserve exact values and reject overflow or excess precision. The Testnet collateral Move type comes from Testnet metadata; Mainnet requires native USDC.

An HTTP order response is an acknowledgment. It does not prove a fill or IOC cancellation. `clientOrderId` is not an upstream idempotency guarantee. Callers must persist their intent before sending and reconcile by the computed order hash. An absent open order or empty trade history cannot establish a terminal cancellation.

Access token validity comes from the login response. Refresh tokens are discarded; obtain another locally signed login when authentication expires. A connected account stream may outlast its token, but reconnecting requires valid authentication.

The protocol follows the [official Pro SDK](https://github.com/fireflyprotocol/pro-sdk/tree/31943cfed2771647b247513596c367a748f5b058) and [API guide](https://bluefin-exchange.readme.io/reference/how-to-use-this-guide).
Sui signing and verification use `onchain/go/sui` within the same module. All
dependencies are already present in `onchain/go/go.mod`.

From `onchain/go`, run `GOWORK=off go test ./venues/bluefin/pro` and
`GOWORK=off go vet ./venues/bluefin/pro`. Tests use injected HTTP/WebSocket
transports and local signature fixtures; they do not submit orders.
