# Uniswap v4 local state quotes

This package owns block-pinned local market-data quotes for hook-free pools with
fixed LP fees below 100%. It adds no dependency on the old Uniswap module and does
not migrate or replace its public HTTP/WS/Quoter APIs. Pools with hooks or dynamic
fees are rejected at construction, even if a specific hook might be harmless.

Compose `NewStateCache(stateRPC, PoolConfig{...}, maxAge)` per pool. The config
contains PoolManager, StateView, ordered currency addresses, LP fee, tick spacing
and hooks. Native currency is represented by the zero address. Pool IDs are
computed from the ABI-encoded immutable pool key. Run `cache.Run(ctx, wsRPC)` in
a supervised goroutine and use `cache.QuotePair(ctx, baseUnits, baseIsCurrency0)`.
The pair independently simulates a base exact-input bid and reverse exact-output
ask on one snapshot; returned amounts include LP and directional protocol fees.

StateView reads slot0, liquidity, bitmap words and encountered tick liquidity at
one block. Packed protocol fees are decoded by direction and combined with LP
fees using `protocol + lp - floor(protocol * lp / 1e6)`. Tick spacing is immutable
configuration. Integer Q96 arithmetic is kept within v4 to avoid changing v3's
verified behavior; v4's partial-step amount rules differ from v3. No floating-point
price calculations are used.

PoolManager notifications are filtered by indexed pool ID, including liquidity,
donation, swap, fee changes and removed logs. These invalidate state rather than
using a trade price as a BBO. A delayed non-removed event at the exact verified
snapshot block/hash is already included. Disconnects and reconnects invalidate;
without a live subscription every pair refreshes. TTL bounds cache reuse when
notifications are missed. It does not guarantee complete event delivery.

When latest is behind a received block, one numbered header lookup can recover
only if the observed number/hash matches. Removed logs and disconnects clear the
candidate hash. All new state reads are followed by canonical header verification;
generation changes during calculation reject the quote. Observation time is
preserved on reuse. Calls use block numbers, so these checks do not claim atomic
cross-request snapshots in a backend that serves inconsistent forks.

Each pair permits 64 contract reads plus header requests and 2048 steps per
direction. Budget exhaustion, insufficient liquidity and inconsistent state return
errors without partial quotes or a Quoter fallback. Busy pools can consume more
RPC than Quoter calls; savings require runtime measurement.

## Retained streaming quotes

Use `RunRetained(ctx, wsRPC, baseUnits, baseIsCurrency0)` with
`QuoteRetainedPair` (or detached `QuoteSnapshot` consumers) for local quotes
without per-quote RPC. The producer preloads the current bitmap word and one
word on each side (center ±1, three words), including every initialized tick
detail in that range, clipped at the protocol tick limits. Losing a required
word, or missing reference-quote tick details, triggers an asynchronous capture
around the new current tick. Large moves do not scan intervening words.

Capture uses StateView with the pool ID and immutable tick spacing. PoolManager
notifications remain filtered by that pool ID. The live inputs remain available
while a new block-pinned window is verified; buffered Swap, ModifyLiquidity and
ProtocolFeeUpdated events after its baseline are replayed before publication.
This preserves directional protocol fees as well as liquidity changes.

Refreshes are serialized with at least one second between captures. Failed
refreshes retain usable inputs and back off up to 30 seconds; initial failures
return to the caller. A capture times out after 30 seconds. The replay buffer is
limited to 4,096 pool logs; overflow, removed logs, and incompatible deltas still
terminate the session for recovery. Uncovered quotes fail until capture catches
up. Retained inputs are not periodically refreshed by TTL.

Three words describe prefetch coverage, not the total RPC budget. Bitmap reads
use one batch; initialized tick details use batches of at most 32, capped at 768
tick reads for the window. Readers without batch support use sequential reads.
All reads include the pool ID and use StateView at the same verified block.
A failed batch rejects the candidate without falling back to individual calls.
Core state, bitmap and extra reference-quote reads retain their 64-call budget;
header checks are additional. Sparse liquidity
can require more words for the reference pair within the existing cap of 64
contract reads. Each capture replaces the old window rather than merging data
from different baseline blocks. Hook and dynamic-LP-fee pools remain unsupported.

Validation: `go test -race ./venues/uniswap/v4` and `go vet ./venues/uniswap/v4`
from `onchain/go`. Injected fakes cover pinned reads, both directions, tick crossing,
fees, TTL/reuse, cancellation, invalidation, reorgs and observed-block recovery.

Protocol references:
- [StateView](https://github.com/Uniswap/v4-periphery/blob/main/src/lens/StateView.sol)
- [ProtocolFeeLibrary](https://github.com/Uniswap/v4-core/blob/main/src/libraries/ProtocolFeeLibrary.sol)
- [SwapMath](https://github.com/Uniswap/v4-core/blob/main/src/libraries/SwapMath.sol)
- [SqrtPriceMath](https://github.com/Uniswap/v4-core/blob/main/src/libraries/SqrtPriceMath.sol)
- [TickMath](https://github.com/Uniswap/v4-core/blob/main/src/libraries/TickMath.sol)
