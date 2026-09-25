# Sui transaction foundation

The `sui` package provides wallet coin reads, programmable transaction construction,
complete unsigned `TransactionData`, local Ed25519 signing, and checked simulation.
It does not select or reserve wallet funds and does not submit transactions.

## Public operations

| Operation | Behavior |
| --- | --- |
| `RPCClient.Coins(ctx, CoinQuery)` | Read one page of real address-owned `Coin<T>` objects with balance, owner, version, and digest. Default / maximum page size: 50. Follow `HasNextPage` and `NextCursor`. |
| `RPCClient.Coin(ctx, ObjectReference)` | Read the current coin and reject a stale or mismatched reference. Callers also check the returned owner, type, and balance against their intent. |
| `ProgrammableTransactionBuilder.SplitCoins` | Split a coin or `ArgumentKindGas` into multiple coins. Select each output using `NestedResult`. |
| `ProgrammableTransactionBuilder.MergeCoins` | Consume the source coins into a destination. |
| `ProgrammableTransactionBuilder.TransferObjects` | Transfer objects to a BCS address argument. |
| `AppendCoinIntoBalance` / `AppendDestroyZeroBalance` | Connect coin funding to existing venue Balance APIs and assert that no input balance remains. |
| `TransactionData.MarshalBCS` / `ParseTransactionData` | Encode / decode complete V1 programmable transaction bytes, including sender, gas payment / owner / price / budget, and expiration. |
| `TransactionData.SigningDigest` | Return the 32-byte user-signature prehash. |
| `TransactionData.Digest` | Return the separate onchain transaction identifier. |
| `ParseKeyPairBase64` / `KeyPair.SignTransaction` | Import the existing Sui keystore Ed25519 format and sign locally. The returned signature bytes are `flag || signature || publicKey`. |
| `VerifyTransactionSignature` | Verify one Ed25519 signature and its expected sender / gas-owner address. |
| `GRPCClient.SimulateTransactionData` | Simulate the exact BCS with checks enabled and automatic gas selection disabled. Return gas costs, input references, balance changes, and events. |

Compose GraphQL reads with `NewRPCClient` and gRPC simulations with `NewGRPCClient`.
The existing quote-oriented `SimulateTransaction` remains available and supports the
new native coin commands. Complete-transaction simulation has a separate method so
that wallet execution checks cannot accidentally inherit quote defaults.

For a compile-tested construction example, see [example_transaction_test.go](example_transaction_test.go).
The caller supplies a current coin reference and an explicit gas budget. For SUI
input funded from gas payment, reserve enough balance for **input amount + gas
budget**, and refer to the gas coin using `ArgumentKindGas` rather than duplicating
it in PTB object inputs. Reserve every coin supplied as gas payment: multiple gas
coins may be merged even when execution fails. See [Sui gas smashing](https://docs.sui.io/develop/transaction-payment/gas-smashing).

Before signing, callers must inspect the decoded transaction and enforce their
allowed packages, functions, assets, recipients, amount limits, and gas policy.
The generic codec validates structure; it does not prove Move function semantics,
balances, ownership, resource usage, or a swap's minimum received amount.

## Encoding and supported scope

- BCS uses protocol enum tags, little-endian integers, canonical ULEB128 lengths,
  fixed-size addresses, and length-prefixed 32-byte object digests. The public Go
  `CommandKind` values retain their existing meanings and are not used as wire tags.
- User signatures use `Blake2b-256([0, 0, 0] || BCS(TransactionData))`. The onchain
  transaction digest uses the distinct `TransactionData::` type prefix. Sources:
  [Sui intent signing](https://docs.sui.io/develop/transactions/transaction-auth/intent-signing),
  [official transaction hashing](https://github.com/MystenLabs/ts-sdks/blob/main/packages/sui/src/transactions/hash.ts),
  [official BCS definitions](https://github.com/MystenLabs/ts-sdks/blob/main/packages/sui/src/bcs/bcs.ts).
- The codec supports Pure / ImmutableOrOwned / Shared / Receiving inputs; Gas /
  Input / Result / NestedResult arguments; MoveCall / SplitCoins / MergeCoins /
  TransferObjects / explicitly typed MakeMoveVec commands; primitive, vector, and
  nested struct type tags; and None / Epoch expiration.
- Address-balance withdrawals, compatibility coin reservations, Publish, Upgrade,
  untyped MakeMoveVec, newer expiration variants, and other signature schemes are
  unsupported and fail closed. Do not assume protocol-wide transaction support.
- Local resource bounds are 1 MiB BCS, 65,536 sequence elements where indexed,
  64 nested type tags, and 4,096 bytes per textual type argument. Network limits can
  be smaller and are checked by simulation / execution.
- `ExpirationEpoch == nil` means **no onchain expiration**. Epoch expiration is not
  a millisecond application TTL. This transaction format does not encode an EVM-like
  chain ID; callers must check the RPC network and apply their network policy.
- Sponsored gas ownership can be represented, but verification checks one signature
  at a time. A caller handling sponsorship must collect and verify all required
  signatures. The initial TradeHub integration can restrict gas owner to sender.

## Coin consistency and simulation

Coin pages are observations, not reservations. Applications should serialize their
own wallet reservations, re-read selected references before preparing, and reconcile
effects before releasing signed / submitted funds. A newer version of the same
object does not constitute a distinct wallet allocation.

Checked simulation never silently substitutes coins or increases the supplied gas
budget. A successful response must echo identical BCS and provide gas-cost fields.
Move failures preserve `SimulationExecutionError` through `errors.As`; transport
errors preserve their chain through `errors.Is`. The result is a prediction, not
proof of signature validity, final execution, or a checkpoint-pinned state snapshot.

## Verification

Run from `onchain/go`:

```sh
go test ./sui
go test ./...
go vet ./...
go test ./sui -run '^$' -fuzz '^FuzzParseTransactionData$' -fuzztime=20s -parallel=2
```

[transaction_vectors.json](testdata/transaction_vectors.json) was generated offline
using the official `@mysten/sui@2.0.0` package. It covers both expiration variants,
all supported input / argument / command forms, nested types, and values above the
JavaScript safe-integer range. Tests compare BCS, the signing prehash, the transaction
digest, and a deterministic Ed25519 signature. Fixture keys and references are
synthetic public test data.

The optional [generator](testdata/generate_transaction_vectors.mjs) takes the path
to an external npm project containing `@mysten/sui@2.0.0`; it writes JSON to stdout
and does not perform RPC calls. Node is not required for Go tests, and no npm
dependency is added to this Go module.

```sh
node sui/testdata/generate_transaction_vectors.mjs /path/to/vector-project > /tmp/transaction_vectors.json
```

On 2026-09-24, a read-only Testnet probe also listed a public owner's real SUI coin,
re-read its exact reference, and simulated splitting 1 MIST from the gas coin back
to the same owner. The public GraphQL endpoint and configured PublicNode gRPC
endpoint accepted the generated 219-byte TransactionData with checks enabled.
Simulation returned one input reference and gas costs of 1,000,000 MIST computation,
1,976,000 MIST storage, and 978,120 MIST storage rebate. These are simulated values;
no private key, signature, submission, or actual fund movement was involved.
The local probe is `/private/tmp/sui-foundation-readonly-20260924.go`; live references
are transient, so it is not part of the deterministic test suite.

TradeHub's Sui Prepare / Submit registration, OMS integration, Agent setup and
durable reservations, and live signed Cetus swaps are subsequent integration work.

### Cetus funded preparation

`RPCClient.CurrentEpoch` returns the current epoch and its reference gas price.
`venues/cetus/clmm.BuildSwapTransaction` accepts explicit resolved `sui.Coin`
objects for funding and returns complete unsigned `TransactionData`. It validates
ownership, types, duplicate references and balances, then merges/splits input,
appends the Cetus swap, enforces full input consumption and minimum output, and
transfers output to the specified recipient. Native SUI input uses GasCoin and
reserves the gas budget; token input uses separate owned coins.

Read the selected coin references immediately before construction, then use
`GRPCClient.SimulateTransactionData`. Neither operation signs, submits, reserves
funds, nor guarantees a checkpoint-pinned result. The caller supplies the epoch
expiration and must coordinate concurrent wallet use.
