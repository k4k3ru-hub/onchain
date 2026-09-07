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
No state cache or local quote arithmetic is introduced in this migration.

Individual constructors remain available for independent HTTP and WS lifecycles:
`NewFactoryClient`, `NewQuoterClient`, `NewPoolStateClient`,
`NewSwapFilterClient`, and `NewSwapSubscriber`. The combined `NewClient` requires
both transports and composes all five groups through explicit dependencies.
Constructor composition and injected-RPC operation tests migrate with the code.

From `onchain/go`, run `GOWORK=off go test ./venues/aerodrome/...` and
`GOWORK=off go vet ./venues/aerodrome/...`.
