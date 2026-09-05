# Adaptive AMM quote snapshots

Raydium CLMM and Meteora DLMM accept `InitialArrayCount` and `MaxArrayCount`
in their owning package's `Config`. Zero selects defaults of 5 and 16.
The normalized counts must satisfy `1 <= initial <= maximum <= 32`.
Counts are total unique array accounts across all requested directions, not
per-direction limits. Pool accounts and Meteora's Clock sysvar are excluded.

For a two-sided quote, the initial window contains the current initialized
array and nearby initialized arrays on each side. Uninitialized arrays are
skipped; when the current array is uninitialized or one side has fewer arrays,
the budget is filled with the nearest candidates in the requested directions.
Single-sided requests use nearby arrays in that direction only.

The quote traversal reads arrays lazily from one `getMultipleAccounts` snapshot.
If the amount is filled, no farther arrays are required. If traversal needs an
unrequested array, it is added to the next request. Every retry refetches the pool,
all selected arrays, and (for Meteora) Clock, then recalculates every quote from
scratch. No balances or partial calculations are combined across snapshots.
Recent pool metadata guides address selection; it does not replace fresh state.
The next independent quote starts with the initial window again.

The total array cap is checked before each request. A quote allows at most five
snapshot requests, including retries caused by pool movement, and honors caller
context cancellation/deadlines. These limits may stop a quote before it uses the
entire supported search range (up to 16 initialized arrays per direction).

Callers can use `errors.Is` with the Solana package's `ErrQuoteArrayLimit`,
`ErrQuoteSnapshotLimit`, and `ErrQuoteArrayRange`. Range exhaustion is not proof
that the pool has insufficient liquidity globally. A requested account returned
as null is an error, not a request to expand the window. Upstream RPC errors are
wrapped and returned without provider switching or arbitrary RPC retries.

Existing constructors and quote signatures remain available. CPMM is unchanged.
The slot-aware batch path applies these budgets; account-only injected providers
remain supported by the single-quote fallback, which does not promise a shared
snapshot. Live RPC clients use the snapshot path.
