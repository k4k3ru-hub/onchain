# CLMM state cache

`NewStateCache(client, pool, maxAge)` caches coherent account state for a
configured pool. It reuses the existing static-fee exact-input calculation and
bounded adaptive tick-array discovery. Dynamic-fee and other unsupported states
retain the existing quote errors; this does not add support for new pool models.

Run `cache.Run(ctx, subscribe)` using a factory returning `AccountChanges`,
for example a wrapper around `solana.WSClient.SubscribeAccountChanges`. Use the
same commitment as the injected account snapshot provider. The owner reconnects
the WebSocket when Run returns an error. Cancellation closes and joins all
subscription workers. There is no logging or implicit global transport in the SDK.

The pool and fee configuration are initially subscribed. A successful quote
snapshot adds its tick arrays to the subscription set. Changes to the required
set replace the session's subscriptions. Every used address must be actively
subscribed before a snapshot can be reused. Notifications, stream loss and
subscription changes invalidate cached state; expiry forces a refresh on the
next quote. No subscription means an RPC snapshot on each call.

Pool, fee configuration and selected tick arrays are fetched together. Independent
notification payloads are not merged. Config bytes are decoded from each new
snapshot rather than relying on construction-time fee metadata. Snapshots are
detached and must not regress behind an observed slot. Notifications arriving
during a quote cause that call to fail rather than publish invalidated state.
A larger requested amount that needs an uncached array triggers bounded adaptive
snapshot expansion and updates the subscription set.

`QuoteExactInputs` evaluates new amounts locally on cache hits and preserves
`CachedQuote.ObservedAt` and the snapshot slot. RPC reduction primarily applies
to quiet periods. Active pools still refresh after changes; silent notification
loss is bounded by `maxAge`, not proven absent. This is state freshness handling,
not complete swap-history recovery.

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
