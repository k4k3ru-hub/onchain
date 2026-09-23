# Token Controls

`controls.NewAnalyzer(codeAnalyzer, stateReader)` accepts a shared
`analysis.Request` / `analysis.Result` verifier and a `CallContractAtHash` reader.
`Analyze` returns typed nullable findings; it neither runs workers nor selects
latest blocks. The application pins the latest block at job start, passes its
hash, verifies canonicality afterwards, and adds the observation time/position.

| Input | Initial observation |
| --- | --- |
| Base token registered by exact onchain definition | Every finding `trusted`, null values, no acquisition |
| Defined native currency | Every finding `not_applicable`, no acquisition |
| Reviewed simple ERC20 / ERC20Permit | No special transfer restriction, upgrade, forced balance operation or runtime mint; ownership not applicable |
| Reviewed WETH9 at an unregistered address | Permissionless mint requires native backing; no dedicated supply cap |
| Restricted Ownable wrapper | Owner read at the supplied hash; zero means renounced within this recognized ownership mechanism |
| Restricted Ownable + onlyOwner mint | Mint mechanism exists; owner nonzero permits mint, owner zero disables it; no backing or dedicated cap |
| Reviewed OniAgent complete runtime | Metadata-only owner read at the supplied hash; no runtime mint or special trading/upgrade/balance authority, including when owner remains nonzero |
| TAOT tax-only model, arbitrary overrides, roles, proxy or other code | Unknown, never inferred false |

This identifies contract capabilities, not whether a particular wallet's next
transaction will execute. A nonzero owner can be another contract whose callers
have their own constraints. `canMint` reports the token's permission path; it does
not attest to the holder's keys, liveness or intent. Constructor issuance does
not count as a runtime mint mechanism. Numerical uint256 limits are not a
dedicated supply cap.

Owner errors preserve independent code findings and return an inspectable error;
owner-dependent values remain null. `renounced` does not require a historical
renounce event. It requires the supported ownership structure, zero owner at
the pinned block and no accepted recovery route. It is not a global safety claim.

TokenTaxes remains separate. Trusted is policy-based omission, not a finding of
zero tax or absent permissions. Controls does not describe LP protection,
holder concentration, a tradable route or ongoing freshness.

`ModelVersion` is `evm-token-controls-v2`. Consumers should invalidate older
persisted Controls results so formerly unsupported OniAgent tokens can be
evaluated again. This package does not emit logs; applications can report
unknown fields using the returned code evidence and nullable observations.
