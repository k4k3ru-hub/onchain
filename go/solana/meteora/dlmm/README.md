# Meteora DLMM state cache

`NewStateCache(client, pool, maxAge)` composes a market-data snapshot cache using
an existing discovered DLMM client. `QuoteExactInputs` returns a `CachedQuote`
with the snapshot slot and `ObservedAt`; it evaluates both directions using the
existing DLMM math, validation, adaptive bin-array limits and fee rules.

```go
cache, err := dlmm.NewStateCache(client, pool, 30*time.Second)
if err != nil { return err }
// Run this session in an owner-managed goroutine; reconnect on returned errors.
err = cache.Run(ctx, func(address solana.Address) (dlmm.AccountChanges, error) {
    return ws.SubscribeAccountChanges(address)
})
```

While Run is active, call `cache.QuoteExactInputs(ctx, requests)` from the quote
worker. Notifications invalidate snapshots instead of merging independently
updated accounts. The subscription set tracks the pool and selected bin arrays;
array expansion reconfigures and closes the previous session. A failed session
invalidates cached state. Without subscriptions, every quote fetches RPC state.

Each refresh captures the pool, Clock sysvar and required bin arrays at one RPC
context slot. Clock participates in fee/activation calculations but is not
subscribed, so each new chain slot does not invalidate otherwise reusable data.
Cached quotes describe the original snapshot and Clock, not the current slot;
`ObservedAt` is not advanced on cache hits. At maxAge, refresh includes Clock.
Use fresh `Client.QuoteExactInputsWithSlot` for execution quotes.

A newer notification racing calculation rejects the result. RPC snapshots older
than the observed slot are rejected. Silent subscription delivery failures can
still leave state old until maxAge expires; the cache is not a completeness or
transport-health guarantee.

### Retained range refill

Successful initialization retains the reference requests. `Run` uses a separate
producer to inspect the latest retained pool and nearby array identities once per
second, without RPC. It refills a changed/incomplete initial array range before
the reference quote needs to cross into it. Missing reference arrays also trigger
refill. The existing `InitialArrayCount` and `MaxArrayCount` govern acquisition;
all tick/bin details in selected array accounts are fetched in account batches.

A refill has a 30-second timeout and retries with 1–30 second backoff. Subscription
updates continue during IO. Failed captures preserve retained state; successful
captures merge by slot, preserving equal-slot/newer live payloads and receipt
times, then replace the subscribed address set. Departed arrays are pruned.
`QuoteRetainedExactInputs` and detached `QuoteSnapshot` calculations remain local
and do not initiate refill or wait for it. These retained inputs do not assert
cross-account atomicity. The separate coherent `QuoteExactInputs` API still owns
its explicitly requested RPC behavior.
