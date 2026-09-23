# ERC-20 TokenTaxes

`tax.NewAnalyzer(reader, sources, compiler)` composes fixed-block code reads,
public-source retrieval and a caller-supplied Solidity compiler. The analyzer is
synchronous; MarketHub owns its worker pool, shared acquisition budgets, caches,
observation timestamps, canonicality checks and database updates.

Initial scope is **Base mainnet**. An observation describes token transfer taxes,
not pool fees, gas, price impact, sale permission or liquidity protection.

## Supported models

| Input | Observation |
| --- | --- |
| Explicit native currency in onchain definitions | Buy/sell `"0"`, tax changes/exemptions `false`, source `native_currency` |
| Defined Base USDC address | Nil observation, `trusted_token_skipped`; no code/source/compiler calls |
| Reviewed complete WETH9 or TAOT runtime hash | Buy/sell `"0"`, tax changes/exemptions `false`, source `contract_analysis` |
| Reviewed OpenZeppelin dependency contents with an ERC20 or ERC20Permit wrapper | Same zero-tax observation after source-model recognition and runtime matching |
| Unknown proxy, additional methods/overrides, changed dependencies, unsupported compiler or unmatched code | Nil observation with an internal reason |

Source recognition accepts one concrete wrapper inheriting ERC20 or ERC20Permit,
with a parameterless, nonpayable constructor, literal token names/symbols and an
optional `_mint(msg.sender, numeric_literal)` statement. It accepts new contract
names, symbols and supplies without registering token addresses. It currently
rejects additional wrapper methods, constructor parameters, other base contracts,
custom modifiers and arbitrary constructor expressions. This is a deliberately
limited model, not a general Solidity semantic analyzer.

Imported contents must match the reviewed OpenZeppelin bundle hashes in
[`../analysis/reviewed.go`](../analysis/reviewed.go); filenames and ABI getters alone cannot establish tax behavior.
The wrapper AST and every dependency come from the supplied local compiler output.
The `Compiler` implementation is a trusted application dependency, not a remote
source provider's `stdJsonOutput` field.

`CanChange` concerns tax/application changes only. In particular, the TAOT runtime
still has mint/bridge roles; a zero-tax observation does not describe those roles.
Rates and flags are independent pointers so later models can preserve partial
unknowns. Initial supported models establish all four fields. Unknowns are never
converted to zero or false.

## Code and compiler evidence

- `Request.BlockHash` is required. `evm.HTTPClient.CodeAtHash` performs one read at
  that exact hash, without falling back to latest. The caller verifies chain and
  canonical block identity and retains the matching block number/timestamp.
- Complete reviewed runtime hashes are checked before source retrieval.
- If the original compiler is locally available, it is used first. Otherwise up
  to two available versions are tried in descending version order, followed by
  the original version. The compiler checks the unmodified source pragma.
- A successful match with a bundled version ends processing without acquiring
  the original version. `OriginalCompiler` and `UsedCompiler` remain distinct.
- The adapter must return compiler output from the exact input/version, enforce
  process limits, verify executable hashes and classify unavailable/failed candidates
  with `ErrUnsupported` or `ErrCompilation`. The analyzer checks JSON diagnostics and required artifacts;
  exit code zero does not establish successful compilation.
- Executable code must match. Only reviewed EIP712 domain-cache/name/version
  immutable declarations are eligible for substitution, with bounded,
  non-overlapping ranges and consistent values for repeated references.
- Metadata differences are allowed only for recognized source models, complete
  known Solidity CBOR maps and an equal executable body without code-inspection
  opcodes. Unknown metadata is kept in the comparison. Library links are rejected.
- Code/model identity and compiler versions are returned as internal evidence.
  This package does not issue timestamps or populate a MarketHub JSON response.

`prepareInput` accepts embedded source content, limits input to 2 MiB and requests
only AST/runtime/method identifiers. Original semantic compiler settings are
preserved. Source URLs and model-checker execution are unsupported. Compiler JSON
output is capped at 8 MiB. These bounds complement the application's native runner's
CPU/memory/time/output limits; they do not provide process isolation themselves.

## Source retrieval and composition

The owning adapter package is
`github.com/k4k3ru-hub/onchain/go/evm/erc20/tax/sourcify`.
`sourcify.NewClient(httpClient, "https://sourcify.dev/server")` implements
`tax.SourceProvider`. Pass it, the EVM reader and the application's native
`tax.Compiler` into `tax.NewAnalyzer`.

The adapter uses GET `/v2/contract/{chainId}/{address}` with
`fields=compilation,stdJsonInput`, checks the returned identity, and limits each
request to 20 seconds and its body to 4 MiB. It never submits verification jobs or
reads third-party tax verdicts. The caller's HTTP client owns transport and
redirect policy.

HTTP 4xx/5xx and transport failures remain inspectable errors. The adapter makes
one attempt and does not log response bodies. MarketHub must share successful
reads and apply initial attempt plus at most three backoff retries, including
4xx; it must not multiply retries across layers. Compiler/model mismatch is
separate from acquisition failure.

The existing ERC20 client constructor remains unchanged. No environment lookup,
package-global replaceable dependency, background worker, process invocation or
import from `experiments` is used by this package.

## Fixtures and verification

Fixtures are local inputs/evidence, not runtime dependencies:

- `weth9-runtime.json`, `taot-runtime.json`, `permit.json`, `sourcify.json`: saved
  Base block 51,626,749 (`0x968b022a36e55f34a2b512d07ae82ca232975272428d3064254c12181aa2d96f`)
  from the [earlier investigation](../../../../experiments/token_taxes_20260922/README.md).
  `permit.json` uses local 0.8.37 output and actual saved token runtime.
- `simple.json`, `taxed.json`, `getter.json`: independently prepared wrappers
  compiled locally using checksum-verified native 0.8.37. The latter two compile
  successfully but must not qualify for the limited tax-free model.
- `simple-0834-runtime.json`: the same independent token compiled with 0.8.34;
  tests match it against 0.8.37 output and assert no original compiler acquisition.
- OpenZeppelin source contents retain SPDX headers. Their license is reproduced
  in [testdata/OPENZEPPELIN_LICENSE](testdata/OPENZEPPELIN_LICENSE).

Local fixture generation uses the isolated compiler image from the
[runtime investigation](../../../../experiments/token_taxes_runtime_20260922/README.md),
but package tests require neither Docker nor research files. The independent
fixture runtime is locally generated evidence, not a new onchain deployment.
This implementation step uses zero additional onchain RPC calls.

Run from `onchain/go`:

```sh
go test ./evm ./evm/erc20/tax/...
go test -race ./evm/erc20/tax/...
go test ./...
go vet ./...
```

`native.NewCompiler(runtime)` adapts an application-owned native runtime through
the `Available(ctx)` and `Compile(ctx, version, input)` interface. The server now
provides a formal native runner, official acquisition/cache and Docker packaging;
the SDK does not import that server module. The Linux `integration` test invokes
the formal `solc-manage` executable using `SOLC_MANAGE_PATH`, `SOLC_HELPER_PATH`
and `SOLC_BUNDLE_DIR`. It verifies both accepted/rejected models and an actual
0.8.34 runtime matched with 0.8.37 output without acquiring the original compiler.

MarketHub owns worker/snapshot integration; SDK Pair fields and Agent/Console
display consume the resulting observations. Production code does not
invoke research scripts. The process runner's resource-limit tests are independent
of the earlier research results.

References: [Sourcify v2](https://docs.sourcify.dev/docs/api/),
[Solidity metadata](https://docs.soliditylang.org/en/latest/metadata.html).

## Shared Controls analysis

The code/source/compiler pipeline now lives in [`analysis`](../analysis/README.md).
The existing `tax.NewAnalyzer` constructor and public request/provider/compiler
types remain supported. `tax.NewAnalyzerWithCode(verifier)` accepts a shared
`analysis.Request` / `analysis.Result` verifier so an application can cache code
verification across Tax and [Controls](../controls/README.md). Controls Ownable
models do not expand the tax model whitelist; their tax result remains null.
