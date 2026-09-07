# Uniswap v3

This package owns the migrated Uniswap v3 HTTP/WS clients, QuoterV2 calls,
Swap parsing/filtering/subscriptions, slot0 reads, pool keys, and deployment data.
It is copied from `uniswap/go/v3` without behavior changes; imports now use
`github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3`.

Import `.../v3/protocol` for pool keys and currencies and `.../v3/deployment` for
deployment metadata. There are no root aliases or dependencies on the old v3
package. Existing users of `uniswap/go/v3` can continue using that package;
its named types are distinct and must not be mixed with the new package types.
Uniswap v4 remains in its existing repository.

## Composition

```go
import (
    v3 "github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3"
    "github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
    "github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/deployment"
)
```

Use `v3.NewHTTPClient(v3.HTTPClientParams{RPC: httpRPC, Factory: factory})`
and `v3.NewWSClient(v3.WSClientParams{RPC: wsRPC, Factory: factory})` for independent
lifecycles, or `v3.NewClient` to compose both. RPC interfaces remain injectable.
`factory` is a `v3.FactoryConfig`; its pool keys are `protocol.PoolKey` values,
and `deployment.ByChainID` supplies supported deployment addresses.

Run `go test ./venues/uniswap/v3/...` and `go vet ./venues/uniswap/v3/...`
from `onchain/go`. Existing constructor, quote, event, and deployment tests moved
with the implementation. State caching and local tick-crossing quotes are a
subsequent change; this migration still calls QuoterV2 for quotes.
