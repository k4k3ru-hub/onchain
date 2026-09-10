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

### Retained quote receipt times

EVM Uniswap v3/v4 and Aerodrome Slipstream, and Sui Bluefin, Cetus, Turbos and
Momentum expose `ReceivedAt` on retained quote results and
`QuoteSnapshot.ReceivedAt()` on detached inputs. It is the local observation
of inputs, preserved across repeated calculations and ignored replayed events.
Solana Raydium CPMM/CLMM and Meteora DLMM also expose `CachedQuote.ReceivedAt`,
derived from the last accepted account observation; their `ObservedAt` remains
the oldest contributing account observation.
`CapturedAt` still describes calculation capture; block/checkpoint timestamps
retain chain provenance. Consumers measuring input freshness should use
`ReceivedAt`, not `CapturedAt`. A nil snapshot accessor returns a zero time.

```go
snapshot := cache.CaptureQuoteSnapshot()
if snapshot != nil {
    receivedAt := snapshot.ReceivedAt()
    _ = receivedAt // Preserve with the BBO derived from this snapshot.
}
```

### Retained AMM range acquisition

Initial acquisition reads complete calculation details for the selected native
range. Live retained quote methods never issue RPC, even when coverage is missing.
Range refill belongs to the state producer, not Swap or quote consumers.

| Venue | Acquisition unit | Retained producer behavior |
| --- | --- | --- |
| Uniswap v3/v4, Aerodrome Slipstream | Current bitmap word ±1; every initialized tick | Batch capture and live-update replay |
| Bluefin, Momentum, Turbos | Current bitmap word ±1; every initialized tick | Checkpoint-pinned bitmap batch, tick batches of 32, asynchronous capture and transaction replay |
| Cetus | Current compressed-tick interval ±1 (256 positions per interval) | Checkpoint-pinned keyed batches of 50; all initialized ticks in range; asynchronous recenter and transaction replay |
| Raydium CLMM | Complete tick-array accounts | Existing `InitialArrayCount` / `MaxArrayCount`; asynchronous refill as nearby array identities change |
| Meteora DLMM | Complete bin-array accounts | Existing `InitialArrayCount` / `MaxArrayCount`; asynchronous refill as nearby array identities change |
| Raydium CPMM | Pool/config/reserve accounts | No tick range; retained full-account updates |

Bluefin/Momentum/Turbos three-word capture needs one bitmap query and up to 24 tick queries (32 keys
per query, at most 768 initialized positions). This excludes pool/checkpoint,
protocol configuration and any extra inputs needed for the reference quantity.
Empty bitmap words require no tick-detail query. Network batching does not imply
that a provider bills the batch as one logical operation.

Sui range captures continue receiving/applying live updates. Candidate state is
installed only after replay and pool-version/checkpoint checks; failed capture,
replay or regression preserves the previous live state. Replay is bounded to
4096 transaction notifications. Solana merges captured full accounts by slot,
retains equal-slot/newer live observations, prunes departed addresses and
reconfigures array subscriptions. Neither model asserts cross-component atomicity.
Both producers inspect coverage locally once per second, use a 30-second capture
timeout, and back off failed captures from one to 30 seconds, resetting on success.

Cetus uses u64 skip-list keys, not bitmap words. Capture enumerates only the
aligned tick positions in the selected three-interval window, at most 768 keys
in 16 sequential GraphQL batches of 50 (plus pool/checkpoint reads). Missing keys
prove empty positions at that checkpoint. No global dynamic-field enumeration or
full-scan fallback is used, including initial capture and reconnect. This bounds
work per pool; it does not remove aggregate load as more pools start, nor promise
fewer queries than a full scan of a very sparse pool. Provider billing may count
each key separately. Current-tick interval changes trigger asynchronous recentering;
quotes beyond verified bounds return `coverage=insufficient` without RPC or a
partial fill. Outside-window streamed tick changes are ignored. Solana's existing
account-based price provenance is unchanged by this range-management work.
