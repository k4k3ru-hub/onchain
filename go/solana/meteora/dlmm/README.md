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
