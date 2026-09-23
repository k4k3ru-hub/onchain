# LP principal protection

`Reader.Analyze` starts with a PoolCreated / V4 Initialize event and an existing
complete `clliquidity.Snapshot`. It discovers NFT positions from pool Mint or
V4 ModifyLiquidity receipts, verifies their current state, recognizes reviewed
custody runtime, and calculates
Token0 / Token1 principal protection independently. No UNCX API, locker-address
allowlist, research package, Agent, source download or compiler is required at
runtime. Unrecognized code remains unresolved.

This is the onchain component only. MarketHub workers, persistence, invalidation,
public SDK JSON, Agent and Console integration are separate implementation units.
LP protection does not determine NewPair listing eligibility.

Model `lp-protection-20260924-v5` retains verified creation boundaries and reusable
contiguous operator history, rechecks the existing prefix before extending it,
and invalidates evidence on observation reorgs. Validated v4 evidence migrates
without resetting acquisition failures. Versions before v4 remain unsupported;
callers must stop unsupported state rather than silently reset its retry ledger.
V3 / Slipstream contract custody still requires complete operator evidence.
The reviewed V4 LaunchLocker uses separate creation/authority evidence instead.
See [the evidence API and limited creation rule](EVIDENCE.md).

## Scope and trust boundary

- Base mainnet Uniswap V3: factory `0x33128a8fC17869897dcE68Ed026d694621f6FDfD`,
  position manager `0x03a520b32C04BF3bEEf7BEb72E919cf822Ed34f1`.
- Base mainnet current Aerodrome Slipstream: factory
  `0xf8f2eB4940CFE7d13603DDDD87f123820Fc061Ef`, position manager
  `0xe1f8cd9AC4e4A65F54f38a5CdAfCA44f6dD68b53`.
- Base mainnet hookless Uniswap V4: official PositionManager
  `0x7c5f5a4bbd8fd63184577525326123b519429bdc`. Ordinary EOA withdrawal and
  creation-verified LaunchLocker permanent custody are supported. See
  [V4 scope, evidence and verification](V4.md).
- These infrastructure deployments and Base voter
  `0x16613524e02ad97eDfeF371bC883F2F5d6C480A5` are trusted protocol boundaries.
  Factory event, canonical chain, manager/factory/pool bindings and live core
  position balances are checked. This does not audit arbitrary managers or
  voter implementations. Legacy Slipstream and other chains are unsupported.
- RPC must be trusted. The caller must preserve the originating pool and block
  of a complete tick snapshot, and provide immutable request/cache data while an
  analysis is running. `Principal.Pool` is required; the current price, tick and
  liquidity are additionally compared with the pool at the observation hash.
  For V4, `Pool` is the PoolManager; `Principal.PoolID` is also required.
- State and runtime calls use the observation **block hash**, never `latest`.
  The canonical header is checked before/after analysis. Every reused receipt
  must match its transaction, block and input event; removed logs are rejected.

## Meaning of results

`Observation == nil` means there is **no confirmed pool-wide percentage**.
`Reason` distinguishes unsupported deployment/custody, incomplete coverage,
no principal, acquisition failure and budget exhaustion. Wrapped RPC errors remain
inspectable. Partial positions and discovered IDs are diagnostic evidence, not
partial confirmed percentages.

Each token's denominator is that token's principal across **all live positions**.
Fees, direct transfers, and removed-but-uncollected balances are excluded. Full
tick gross/net coverage is compared with enumerated positions, then the manager's
core position liquidity is checked for every range (and each NFT salt in V4).
Active liquidity alone, NFT counts, or unweighted liquidity totals cannot
establish completeness.

Position principal uses exact rational Q96 arithmetic before percentage flooring
to 18 decimal places. It describes unrounded principal shares, not an executable
integer withdrawal quote. For one token with zero principal its percentages are
nil. With neither token present there is no observation.

`AllPositionsProtected` is derived from every position's classification, not from
rounded percentages. Tiny open positions prevent a true result. Temporary locks
have an `EarliestUnlockAt`; Unix `4294967295` is a finite date in 2106, **not**
permanent protection. V4 LaunchLocker is the limited permanent-protection model;
see [its proof requirements and verification](V4_PERMANENT.md).
Zero/dead addresses, delegated EOAs and unknown contracts do not imply a burn.

`CanWeakenProtection` describes the reviewed authority paths separately from
current custody. It may be nil even when current percentages are available.
An already configured but unreviewed migration/gauge path prevents a positive
current-lock verdict. Token taxes, minting, upgrade rights and transfer controls
are outside this result and must be evaluated separately.

## Reviewed custody models

| Model | Requirements and result |
| --- | --- |
| Direct ordinary EOA | Current owner has empty code, is not a reserved/burn address, and full `decreaseLiquidity` succeeds from that owner at the observation hash. Currently withdrawable. No claim about private-key availability or subsequent token transfer success. |
| V4 ordinary EOA | Reviewed official PM runtime; complete NFT/core/tick coverage; full `modifyLiquidities` decrease plus settlement of both currencies to the actual owner succeeds. An `eth_call` at the observation hash, without signing or state overrides. |
| V4 LaunchLocker | Reviewed locker/factory/PM runtime, exact signed first-deployment initcode and immutable bindings, matching creation runtime and canonical anchors, no observed authority conflict. Permanent protection with no unlock deadline. |
| MultiVault runtime | Exact normalized runtime and consistent immutable values; no minted vault key; immutable beneficiary is an ordinary EOA. Future unlock timestamp, still-locked state and complete operator evidence establish temporary protection. After expiry, the actual beneficiary's `partialNonFungibleTokenUnlock` path including factory callback must succeed. |
| CL locker clone + factory | Exact EIP-1167 clone, reviewed implementation and reviewed factory runtime; immutable manager/voter/factory/implementation bindings; live instance, pool and NFT match; unstaked, unapproved and complete operator evidence. Before expiry, migration must be disabled and both saved/current pool gauge absent. Factory owner grants a future migration configuration path, so current 100% can coexist with `CanWeakenProtection=true` only after all custody checks pass. Zero factory owner alone does not prove absence of voter governance powers. |

All positively classified contract-owned positions require zero individual NFT approval. Nonzero
approval is unresolved, including a contract spender that cannot be assumed to
act just because `eth_call.from` can impersonate it.

The reviewed V3 / Slipstream Vault/clone runtime has no `setApprovalForAll` path, but this does
**not** exclude existing approvals: a different constructor can set operator
approvals before returning identical runtime. Therefore positive lock verdicts
require the trusted NFT manager's complete `ApprovalForAll` history for the
custodian, from block zero or a verified first creation through observation,
followed by `isApprovedForAll`
for each discovered operator at the observation hash. Before-pool approvals are
included. Active operators return `operator_approval_active`; incomplete evidence
returns `operator_history_unresolved`, never a confirmed percentage or a claim
that protection cannot be weakened. Operator acquisition progress is returned as opaque `Evidence` and can be
reused across analyses; current approval values are re-read at each observation. The canonical managers' approval storage and event semantics
are part of the infrastructure trust boundary.

Before scanning, the reader checks whether all log ranges fit the remaining RPC
budget. With 1,000-block ranges and a 128-call budget, Base's entire history does
not fit. Without a verified creation boundary or valid saved history, the
reader returns unresolved without issuing those history requests. The optional
CL Factory / CreateX / CREATE clone rule establishes a nonzero start only after
verifying the deployment transaction, constructor fingerprint, address derivation,
runtimes and birth-block nonce range. Candidate transaction hashes alone do not
prove creation. MultiVault creation shortening remains unsupported.

Clone NFT approval is restricted to its gauge staking path, which this first
rule excludes. Vault
reinvestment accepts an external manager argument, so the ERC20/ERC721 `approve`
selector collision was also reviewed: a positive `safeApprove` amount first
requires ERC20 `allowance`, absent from the supported NFT managers; zero would
approve nonexistent NFT ID 0. Do not extend this rule to another manager without
rechecking those assumptions. Unknown code containing operator/delegatecall/
approval paths cannot match these fingerprints.

Changes to immutable words are permitted only at compiler-listed offsets, and
all repeated occurrences must agree. Every other runtime byte, including compiler
metadata, must match. Locker names and addresses do not select the model.
Source verification is deliberately bounded; this is not a universal contract
audit or a guarantee against all token/protocol failures.

### Evidence provenance

Fingerprint/runtime evidence was retained from the reviewed
[September 21 research cohort](../../internal/experiments/lplock/VENUE_SAMPLING_30.md).
Production code does not import that package. Test fixtures contain observed
public runtime bytecode, not full third-party Solidity source. Source licenses
remain as recorded in the research archive (MultiVault MIT; CL locker/factory
BUSL-1.1).

| Template | Solidity target / compiler | Archived source response SHA-256 | Normalized runtime SHA-256 |
| --- | --- | --- | --- |
| `vault` | `contracts/locker/vault/MultiVault.sol:MultiVault`, 0.8.4 | `86d4f1239c6526657db4d0325312558d3b3729828d0e65b297e40c93d5a49843` | `bc1e63cbeb10aebceb42eb55442da017e4dc698951a4f1471af89d931666113d` |
| `clLocker` | `src/extensions/cl/CLLocker.sol:CLLocker`, 0.8.30 | `818618407fcb39a4b50f02631d9ca3c1037cb04d563f84a9f2b4240994c09fc9` | `3e837d26af2a2069d0cb675d34030aac097e4ef1db05eff173fd1fbc5461e89c` |
| `clFactory` | `src/extensions/cl/CLLockerFactory.sol:CLLockerFactory`, 0.8.30 | `d6e3b150882b6eb7b7fcb4a6e8a14b0ddcd181e64797b5e0a5c04fc99506e8c1` | `1cb81f8b24806d4f55e69cdac3918814e41f945ad65e23af9ae5de693b2481b4` |

Compiled and observed runtime normalize to the same digest. Immutable names in
`templates.go` correspond to the source declarations: Vault factory/key/initial
beneficiary/deployer/creation timestamp; CL locker voter/factory/reward token/
pool type/root/manager; CL factory voter/pool launcher/implementation/pool type/
manager/pool factory. `Result.Contracts` retains observed code hashes, while each
position records the applicable model and unresolved reason.

## Acquisition and reuse

Construct the reader with explicit dependencies and limits:

```go
reader, err := lpprotection.NewReader(rpcClient, lpprotection.Limits{
    Timeout: 2 * time.Minute, MaxCalls: 128, MaxReceipts: 64,
    LogBlockRange: 1000, MaxLogs: 4096, MaxPositions: 256,
    MaxResponseBytes: 4 << 20,
})
```

`rpcClient` must implement the package's small `RPC` interface; an
`ethclient.Client` works directly. The reader performs no environment lookup or
dependency construction in request methods. There are no new module dependencies.

Each RPC method invocation permits **one actual attempt**. The reader has no
automatic retries. Application retries must share the approved 128-send / 2-minute
budget, including the initial attempt and up to three retries. Approval-range
failures are retained in `Evidence`; after four failed attempts the range stays
abandoned across restore and later observation blocks. Other acquisition retries
and their failure ledger remain application responsibilities. Do not hide retries
inside this RPC interface, discard failure evidence, or present several bounded
evaluations as one evaluation that met the two-minute limit. Transport caches or a more capable
shared scheduler must account for their own actual sends when adapting it.

Receipt budgets count distinct additional receipts, including creation receipt
verification, and exclude request-local cache hits. Method counts count outward
interface calls; `ResponseBytes` counts decoded call/code/log payloads, not HTTP
framing or JSON bytes. A transport must separately limit wire response sizes.

Each approval range uses two fresh tail headers, its log query, and a fresh
prefix (or verified birth) header before its checkpoint advances: four calls,
except the first range starting at genesis which needs three. The preflight
budget includes these verification reads. A final observation hash mismatch
also clears reusable evidence, even when all history queries succeeded.

`Request.MintLogs` and `Request.Receipts` reuse already received data. The reader
also discovers positions from the creation receipt already acquired for identity
verification. All cached evidence is validated before use. If these receipts or
supplied `Result.PositionIDs` fully explain current ticks, no Mint history request
is made. Otherwise Mint history stops as soon as complete coverage is established.
Discovered IDs survive later acquisition failures for subsequent reuse; their
mutable state is always re-read. Runtime/getter/
receipt repeats within an evaluation are cached at the fixed observation hash.
Creation transaction / receipt / historical code / nonce responses also survive
in bounded `Evidence`; canonical headers are rechecked before reuse. Restored
evidence must come from trusted internal storage, never an API request.
There is no global cache or background goroutine.

When a cached receipt's block differs from the supplied event, discard that
receipt and its dependent creation/history evidence before reacquiring it.
The replacement must match the full event, and uses the same call/receipt budgets;
there is at most one actual receipt request per transaction per analysis. A
failed replacement stays unresolved and does not restore the stale proof.
Unrelated history retry counts are preserved. Callers should also replace stale
entries in their own `Request.Receipts` cache; the reader never mutates it.

The first implementation treats an unreadable/burned historical NFT record as
unresolved acquisition; it does not silently discard a revert as a burned NFT.
Applications may supply a validated current index to avoid stale IDs. Resolving
burns and smart-wallet custody more broadly is a later model/index extension.

No LP-specific polling, retry scheduler, expiry timer or event subscriber lives in
this SDK. MarketHub must invalidate old observations on relevant events, reconnect,
restart, reorg and expiry, then run bounded evaluation. The original principal
snapshot acquisition is a separate, already shared cost.

## Verification

```sh
go test ./evm/lpprotection ./evm/clliquidity
go vet ./evm/lpprotection ./evm/clliquidity
ONCHAIN_LP_PROTECTION_LIVE=1 go test ./evm/lpprotection -run TestLPProtectionLive -count=1 -v
```

The live test reads two frozen Base pools at block `51596896`
(`2026-09-21T09:32:19Z`), starts from creation events, separately measures reusable
principal setup and additional protection reads, and repeats the EOA case with
discovered IDs. The contract-custody case must remain unresolved when operator
history cannot be covered within budget; its old v1 positive result is withdrawn.
It sends no transaction and is not a current NewPair / Agent E2E test.
Public RPC calls are spaced 2.5 seconds apart with a 1,000-block log range.

The opt-in production creation-rule check is:

```sh
ONCHAIN_LP_CREATION_LIVE=1 go test ./evm/lpprotection -run '^TestCreationRuleLive$' -count=1 -v
```

It uses actual bounded log queries and normal RPC calls, with no research verdict
adapter. A cold timeout stays unresolved. A separately measured later evaluation
can use the retained progress; each evaluation retains the same limits.

## Injected deployment candidates

`NewReaderWithCreationResolver(rpc, creationRPC, resolver, limits)` adds optional
`CreationHintResolver` discovery. Existing constructors and `Request.CreationHints`
remain available. Supplied hints and persisted creation evidence take precedence.
The resolver is invoked only after a reviewed custody model identifies the factory
and a matching lock-creation event. It shares `Analyze`'s deadline.

`evm/contractmetadata/sourcify.NewClient` retrieves the untrusted deployment
transaction hash through Sourcify v2 (`fields=deployment`). Its injected HTTP client
makes one attempt, with a 20-second timeout and a 64 KiB response bound. The
application owns shared caching, persistent retry counters and the HTTP budget.
Neither a candidate nor a Sourcify match status proves lock protection: the reader
still verifies transaction, receipt, bytecode, address derivation and chain state.

`errors.Is(err, ErrReorg)` identifies a revoked block/proof. The result reason is
`observation_reorg`; applications must clear the revoked public observation,
including any previously retained percentage. Ordinary acquisition failure may
retain an older valid observation as explicitly stale data.
