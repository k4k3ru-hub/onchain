# Verified quote inputs

AMM StateCache implementations expose `CheckState(ctx)` to read coherent
calculation inputs without executing a swap calculation. `Check.Key` is opaque
and comparable only within the same cache. An unchanged key permits retaining a
previously successful quote. `Position` identifies the verification block,
checkpoint or slot; `CheckedAt` identifies acquisition start. Neither replaces
that quote's original state provenance.

`CheckCurrent(check)` verifies that subsequently observed changes do not invalidate
that check. Consumers must recheck before confirming a publication, invalidate on
failures, and calculate again after failure recovery. A read set can expand when
another requested quantity crosses additional ticks; its key then changes
conservatively. Never compare keys from different pools or caches.

EVM and Solana caches expose one-consumer, coalesced `Changes()` channels alongside
existing `Run` subscription lifecycles. Sui callers can compose
`GRPCClient.SubscribeObjectTransactions(ctx, pool)` and pass transaction checkpoints
to the Quote cache's `ObserveCheckpoint`; Trade cursors remain separate. Progress
watermarks are not quote-state proofs. Missed events still require periodic state
verification. Reconnects must trigger verification even with no matching trades.

EVM block hashes, Sui checkpoint/object identity and Solana account context remain
validated. Inputs include the consumed tick/bitmap/array state and fee settings.
Meteora includes chain Clock, allowing time-dependent quotes to change with no Swap.
These checks perform state RPC reads; they are not a zero-RPC or rate-limit guarantee.

Implementations are in `venues/uniswap/{v3,v4}`, `venues/aerodrome/slipstream`,
`venues/cetus/clmm`, `venues/bluefin/spot`, `venues/momentum/clmm`,
`venues/turbos/clmm`, `solana/raydium/{cpmm,clmm}`, and `solana/meteora/dlmm`.
