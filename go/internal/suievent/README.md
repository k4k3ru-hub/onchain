# Local Move event decoding

The combined Sui `SubscribeTransactions` stream can contain event BCS without
event JSON. On 2026-09-11 the configured node returned BCS but no JSON with each
of `events`, `events` plus `events.events.json`, and explicit event leaf fields.
Requesting JSON explicitly did not fix live Swap decoding.

The Cetus, Bluefin, Turbos and Momentum live parsers therefore decode BCS locally
when JSON is absent, then use the existing venue JSON validation. Present but
malformed JSON remains an error. No extra RPC, subscription, or layout lookup is
performed by these parsers.

The ordered layouts in each venue's `swap_bcs.go` were checked against the
deployed package using `MovePackageService.GetDatatype` on 2026-09-11:

| Venue | Package | Event | BCS bytes |
| --- | --- | --- | --- |
| Cetus | `0x1eabed72c53feb3805120a081dc15963c204dc8d091542592abaf7a35689b2fb` | `pool::SwapEvent` | 153 |
| Bluefin | `0x3492c874c1e3b3e2984e8c41b589e642d4d0a5d6459e5a9cfc2d52fd7c89c267` | `events::AssetSwap` | 158 |
| Turbos | `0x91bfbc386a41afcfd9b2533058d7e915a1d3829089cc268ff4333d54d6339ca1` | `pool::SwapEvent` | 138 |
| Momentum | `0x70285592c97965e811e0c6f98dccc3a9c2b4ad854b3594faab9597ada267b860` | `trade::SwapEvent` | 165 |

Object IDs wrap a 32-byte address; venue I32 wrappers contain a u32. Fixed-width
unsigned integers are little-endian. Every field is consumed, including unused
fields. Truncation, trailing bytes and noncanonical booleans are rejected. Changes
to a deployed event layout require review of its venue decoder.

Live checks confirmed all four parsers accept BCS-only notifications and retain
checkpoint, transaction index and event index. Regression tests compare complete
BCS fixtures with JSON parsing, including u128 values above uint64, and reject
truncated bodies, invalid booleans, trailing bytes and incorrect event types.
