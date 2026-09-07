# CPMM state cache

`NewStateCache(client, pool, maxAge)` adds notification-driven reuse to the
existing CPMM local exact-input calculation. It uses the account snapshot
provider injected into `NewClient`; no global transport or background worker
is created by the cache constructor.

Call `Run(ctx, subscribe)` with an account-change subscription factory. Subscribe
to the pool, AMM configuration and both token vaults at the same commitment as
the snapshot provider. `solana.WSClient.SubscribeAccountChanges` provides the
transport adapter; wrap its return in `cpmm.AccountChanges` in the factory.
The owner closes the WebSocket and reconnects when `Run` returns an error.
Cancellation closes all four subscriptions. Concurrent `Run` calls are rejected.

`QuoteExactInputs` reuses a detached account snapshot only while all four streams
are connected and its age is below `maxAge`. Each notification invalidates the
snapshot. The next quote obtains all four accounts with one `AccountSnapshot`
request, then calculates both directions locally using the existing CPMM fee,
reserve and rounding rules. Multiple callers serialize refresh. Without a
subscription session, each quote fetches a snapshot through RPC.

Account notification payloads are deliberately not merged: independently arriving
pool and vault values need not represent one coherent state. Notifications during
calculation reject that quote. Snapshots behind the latest observed slot are
rejected, including across reconnects. `CachedQuote.ObservedAt` remains unchanged
on cache hits; `Slot` identifies the snapshot used for calculation.

This reduces repeated snapshot requests during quiet periods. It does not remove
RPC refreshes after swaps. `maxAge` also bounds reuse if a live-looking connection
silently misses notifications; it does not prove complete swap-history coverage.
Refresh occurs on the next quote call, not on a dedicated timer. Unsupported token
or pool configurations retain the existing quote validation behavior.
