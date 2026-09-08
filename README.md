# onchain

Go SDK primitives for EVM, Solana, and Sui integrations.

## Venue SDKs

[`go/venues/aerodrome`](go/venues/aerodrome/README.md) provides Aerodrome
Slipstream deployment metadata, Factory and Quoter calls, pool state reads,
and Swap filtering/subscriptions with explicitly injected RPC clients.

[`go/venues/lighter`](go/venues/lighter/README.md) provides public REST and WebSocket
market data for Lighter Core and its independent Robinhood Chain instance. Explicit
deployment constructors select each instance. Separate REST and WebSocket constructors
compose operation groups with manually injected HTTP and WebSocket transports.

[`go/venues/arcus`](go/venues/arcus/README.md) provides Arcus perpetuals market data
through separate REST and WebSocket composition roots, including order books,
trades, candles, market information, and realized/predicted funding.

## Shared pool catalogs

### Robinhood Chain

`core.ChainRobinhood` (`"robinhood"`) resolves to `core.ChainFamilyEVM`.
The EVM registry supports both directions through `evm.ResolveChainID` and
`evm.ResolveChainNetwork`:

| Network | Chain ID constant | Value |
| --- | --- | --- |
| `core.NetworkMainnet` | `evm.ChainIDRobinhoodMainnet` | 4663 |
| `core.NetworkTestnet` | `evm.ChainIDRobinhoodTestnet` | 46630 |

See the [executable example](go/evm/robinhood_example_test.go).
RPC transports remain explicitly injected by callers. This registry does not
register token addresses, deposit policies, or Uniswap deployment contracts.
Applications must also reference an SDK revision containing these definitions
before accepting `chain: robinhood` in their configuration.

Source: [Robinhood Chain network configuration](https://docs.robinhood.com/chain/add-network-to-wallet/).

### Pool references

Use `core.PoolReference` as the stable key shared by market evaluation and
trade execution. Chain-family catalogs own the metadata needed to resolve that
key without coupling applications to one another.

```go
reference := core.PoolReference{
	Chain:    core.ChainBase,
	Network:  core.NetworkMainnet,
	Protocol: core.Protocol("uniswap-v3"),
	PoolID:   poolAddress,
}

pool, ok := evmCatalog.Resolve(reference)
if !ok {
	return fmt.Errorf("failed to resolve evm pool metadata")
}
```

`evm.PoolMetadata.Address` is optional because protocols such as Uniswap v4
identify pools by a bytes32 pool ID under a shared pool manager rather than by
an individual pool contract address.

The `core` package does not import a chain-family package, and family catalogs
do not import protocol packages. Applications compose protocol entries into an
EVM or Solana catalog at their composition root, keeping the dependency graph
acyclic.

## Solana AMM SDKs

Raydium CPMM, Raydium CLMM, and Meteora DLMM clients discover explicitly
configured pools from Solana account state. Public pool and quote types remain
owned by each DEX package.

```go
rpc, err := solana.NewRPCClient(ctx, solana.RPCConfig{
	URL:        rpcURL,
	Commitment: solana.CommitmentConfirmed,
})
if err != nil {
	return err
}

pool, err := solana.ParseAddress(poolAddress)
if err != nil {
	return err
}

client, err := dlmm.NewClient(ctx, rpc, dlmm.Config{
	Pools: []solana.Address{pool},
})
if err != nil {
	return err
}

batch, err := client.QuoteExactInputsWithSlot(ctx, pool, []dlmm.ExactInputRequest{
	{InputMint: baseMint, AmountIn: referenceBaseUnits},
	{InputMint: quoteMint, AmountIn: referenceQuoteUnits},
})
if err != nil {
	return err
}
```

Use `raydium/cpmm`, `raydium/clmm`, or `meteora/dlmm` directly rather than a
root-level facade. All three clients batch mutable account reads with
`getMultipleAccounts`. `QuoteExactInputsWithSlot` evaluates multiple directions
against one account snapshot and returns its RPC context slot in `batch.Slot`.
Use the slot to reject cross-DEX evaluations that do not share the same observed
Solana state. `QuoteExactInputs` remains available when the slot is not needed.

## Solana AMM swap subscriptions

Each Solana DEX package owns its swap event and subscriber types. Compose the
standard Solana WebSocket log client and RPC transaction client explicitly:

```go
ws, err := solana.NewWSClient(ctx, solana.WSConfig{
	URL:        wsURL,
	Commitment: solana.CommitmentFinalized,
})
if err != nil {
	return err
}
defer ws.Close()

subscriber, err := dlmm.NewSwapSubscriber(client, rpc, ws.SubscribeLogs)
if err != nil {
	return err
}
subscription, err := subscriber.SubscribeSwaps(ctx, pool)
if err != nil {
	return err
}
defer subscription.Close()

swap, err := subscription.Recv(ctx)
if err != nil {
	return err
}
```

The subscriber uses `logsSubscribe` for the configured pool and resolves each
successful signature with `getTransaction`. It derives the pool-level net swap
from the two pool vault balance changes. `Transaction.AccountKeys` contains
static keys followed by loaded writable and read-only address-table keys, so
token balance account indexes resolve in Solana runtime order.

One `SwapEvent` represents the net change for one pool in one transaction. If a
transaction invokes the same pool more than once, the event intentionally
aggregates those invocations and uses `EventIndex == 0`.

[`go/venues/uniswap/v3`](go/venues/uniswap/v3/README.md) provides Uniswap v3
QuoterV2, pool-state and Swap HTTP/WebSocket clients with injected RPC dependencies.

[`go/venues/cetus`](go/venues/cetus/README.md) provides the Cetus CLMM Go SDK.
