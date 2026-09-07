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

Validation: `go test -race ./venues/uniswap/v4` and `go vet ./venues/uniswap/v4`
from `onchain/go`. Injected fakes cover pinned reads, both directions, tick crossing,
fees, TTL/reuse, cancellation, invalidation, reorgs and observed-block recovery.

Protocol references:
- [StateView](https://github.com/Uniswap/v4-periphery/blob/main/src/lens/StateView.sol)
- [ProtocolFeeLibrary](https://github.com/Uniswap/v4-core/blob/main/src/libraries/ProtocolFeeLibrary.sol)
- [SwapMath](https://github.com/Uniswap/v4-core/blob/main/src/libraries/SwapMath.sol)
- [SqrtPriceMath](https://github.com/Uniswap/v4-core/blob/main/src/libraries/SqrtPriceMath.sol)
- [TickMath](https://github.com/Uniswap/v4-core/blob/main/src/libraries/TickMath.sol)
