# Pool swap fee observations

`Rates` holds directional trader-facing rates in PPM. `RateString` converts a
validated rate into a normalized decimal fraction without floating point.
Venue packages own `uniswap/v3.DecodePoolCreatedFee` / `ReadPoolFee`,
`uniswap/v4.ReadPoolFees` / `CombinePoolFees` / `DecodeProtocolFee`, and
`aerodrome/slipstream.ReadPoolFees` / `PoolFeeModules`.

Readers receive `ContractReader`, an explicit block and caller condition.
They do not schedule, retry, cache, log, determine listing eligibility, or load
full tick/oracle history. The composition boundary supplies the controlled
transport and verifies block canonicality before publishing observations.

V4 accepts hook-free, fixed-LP-fee Pools only. Its combined fee is
`protocol + lp - floor(protocol * lp / 1_000_000)` with directional protocol
limits. Unsupported hooks/modules return an error wrapping `ErrUnsupported`.
Slipstream accepts published Base modules bound to their registered factory;
its getter includes dynamic/origin-dependent behavior and factory fallbacks.
A zero reference sender is not an assertion that no discount applies.

Deployment references (reviewed 2026-09-22):
- https://github.com/aerodrome-finance/slipstream#deployments
- https://github.com/aerodrome-finance/slipstream/blob/main/contracts/core/CLPool.sol
- https://github.com/aerodrome-finance/slipstream/blob/main/contracts/core/CLFactory.sol
- https://github.com/Uniswap/v4-core/blob/main/src/libraries/ProtocolFeeLibrary.sol
