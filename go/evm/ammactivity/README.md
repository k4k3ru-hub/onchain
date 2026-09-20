# AMM activity decoding

`Decode(venue, log)` normalizes pool Swap and LP principal-change events for
Uniswap V3, Uniswap V4 and Aerodrome Slipstream. It makes no RPC requests and
does not resolve metadata, choose monitoring windows, persist state, or track
canonicality. Those responsibilities belong to the caller.

Quantities are absolute raw `big.Int` token amounts. Swap directions use
`token0_to_token1` / `token1_to_token0`. V4's emitted core swap deltas have the
opposite sign convention to V3; the decoder normalizes this difference.
The amounts describe the pool swap, not the final user's receipts after taxes,
routing or V4 hook adjustments. A zero swap retains the event with no direction.

Mint and positive ModifyLiquidity become `added`; Burn and negative
ModifyLiquidity become `removed`. Zero-liquidity pokes, Collect and Donate are
excluded. Burn quantities describe removed principal, not the later token
withdrawal. V4 ModifyLiquidity does not emit token quantities, so both amount
pointers are nil. Liquidity units are never substituted for token quantities.

Callers must validate the pool/emitter, deduplicate event identities, and apply
removed-log corrections. The function validates event layout and integer widths.

Protocol references:

- [V3 pool events](https://github.com/Uniswap/v3-core/blob/main/contracts/interfaces/pool/IUniswapV3PoolEvents.sol)
- [V4 swap event emission](https://github.com/Uniswap/v4-core/blob/main/src/PoolManager.sol)
- [V4 swap delta construction](https://github.com/Uniswap/v4-core/blob/main/src/libraries/Pool.sol)
