# Frozen quote input metadata

Uniswap v3/v4, Aerodrome Slipstream, Bluefin Spot, Cetus, Momentum and Turbos expose `QuoteSnapshot.Inputs() *quotestate.Inputs`. The method uses the same detached input capture as `QuotePair`, performs no RPC and returns owned maps, ranges and optional indices. `quotestate` owns the metadata types; no root-module aliases are introduced.

```go
snapshot := cache.CaptureQuoteSnapshot()
inputs := snapshot.Inputs() // nil if the snapshot is unavailable
if inputs != nil {
    // Inspect inputs.Baseline, inputs.Position, inputs.Coverage and inputs.ReceivedAt.
    // QuotePair still checks exact quantity coverage locally.
}
```

- `Baseline` is the original complete block/checkpoint. `Position` is the latest applied log or pool-changing transaction; the baseline must not be relabeled as the latest stream position.
- `block` / `checkpoint` means end of that ledger unit. `log` has a block-global log index. `transaction` represents the transaction's resulting pool object; its optional index is checkpoint-local, and zero is valid. Missing metadata remains nil. Sui gRPC transaction and object-state read masks include the existing `transaction_index` field without additional requests.
- Sui fields include pool object version/digest; these are separate from transaction digest/checkpoint. Other fields describe the latest pool square-root price, encoding, tick, liquidity and known fee inputs. Aerodrome reports a dynamic Oracle fee model rather than mislabeling a baseline fee as the current fee.
- `CoverageUnit=bitmap_word` reports actually loaded bitmap keys for Uniswap, Aerodrome, Bluefin, Momentum and Turbos, including known zero words. It does not imply that every initialized tick in each word is loaded. Cetus reports actual retained tick indices (`tick`). Sorted segments preserve gaps. No default acquisition radius, cap or RPC policy changes.
- `UnavailableReason` identifies a known pool pause/lock. Otherwise a frozen calculator may be used, but quantity-specific coverage and fee checks can still fail locally.
- The capture is receiver-side: baseline inputs plus stream replacements. It does not promise a transaction-atomic set of all ticks, bitmap fields and Oracle observations. Receipt time is preserved and never refreshed by metadata inspection or calculation.

Solana price-source and adapter migration remain separate work.
