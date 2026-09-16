# Live execution log decoding

Updated: 2026-09-17.

Raydium CLMM/CPMM `SwapSubscriber.DecodeSwapLog` and Meteora DLMM
`HasSwapInstruction` share invocation tracking. Failed transactions and events
inside failed invocations are excluded. An event is committed only after every
enclosing invocation succeeds. Invocation depths and closing program IDs must
be consistent.

At the first exact `Log truncated` runtime marker, parsing stops. Results from
already completed invocations are returned together with an error matching
`errors.Is(err, solana.ErrExecutionLogsTruncated)`. Events buffered in unfinished
nested invocations and every message after the marker are excluded. Completed
children buffered directly in the outermost invocation are retained when
`Log.Failed` is false: transaction success confirms that outer invocation.
The outer invocation's own unfinished events remain excluded. An unfinished
nested ancestor cannot be confirmed this way because its failure could be caught. An ordinary
`Program log: ...` message containing those words is not a truncation marker.

Callers must distinguish this sentinel from fatal errors. A truncation result
may be empty; it does not imply that any Swap was recovered. Other errors return
no usable result. Existing method signatures are unchanged, and callers that
reject every non-nil error retain their previous fail-closed behavior. Libraries
do not log; applications should report partial acceptance or complete rejection
at their boundary.

For Raydium, retained events still pass pool, payload, mint/direction and amount
validation. Their original log-line `EventIndex`, signature and slot are kept;
the retained subset is not renumbered. No transaction RPC is added. A malformed
matching event remains fatal even when another completed event precedes it.

For Meteora, the boolean result can be true alongside the truncation error only
when a configured-program Swap instruction and all its enclosing invocations
completed before the marker (the outermost invocation may instead be confirmed
by transaction success as above). This is a filter for the existing transaction
resolver, not a quantity decoder. Address mentions and unfinished Swap
instructions do not authorize transaction resolution. The existing resolver's
transaction-level reserve balance normalization is unchanged.

Missing execution log data is not reconstructed. No historical retrieval or
fallback transaction RPC is introduced by partial decoding.


## Transaction retrieval (2026-09-17)

The RPC adapter requests getTransaction with encoding=json and
maxSupportedTransactionVersion=1, consuming static account keys, metadata,
logs and token balances without the legacy SDK binary message decoder.
For v0, loaded writable keys precede loaded readonly keys after static keys.
Legacy and v1 use static keys (v1 has no lookup tables). The v1 transactionConfig
is not needed by the library's Transaction projection. Unsupported returned
versions are rejected. getBlock signature-only requests also allow v1.
Reference: https://solana.com/docs/rpc/json-structures

IsPermanentTransactionError recognizes unsupported versions and invalid
request/method/parameter RPC errors through wrapping. Availability failures
remain retryable. No new dependencies are required.
