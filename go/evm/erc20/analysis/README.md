# Shared ERC-20 code analysis

`analysis.NewAnalyzer(reader, sources, compiler)` composes the synchronous code
verification shared by TokenTaxes and Controls. All dependencies are explicit;
the caller owns RPC budgets, scheduling, retries, caching and canonicality.
`Analyze` requires a fixed block hash and currently supports Base mainnet.

The pipeline checks complete reviewed runtimes first, then obtains source,
compiles it locally, checks reviewed dependencies and a restricted AST, and
matches executable code to the requested block. Compilation success alone does
not establish that a model describes the deployed token. Compiler alternatives,
metadata exclusions and permitted immutable substitutions retain the existing
[Tax verification requirements](../tax/README.md).

`Result.Model` identifies code structure; it is not a tax or permission verdict.
[Tax](../tax/README.md) and [Controls](../controls/README.md) interpret it separately.
Sharing a verified result across blocks requires first confirming identical code;
mutable owner state must still be read at the new block.

## OniAgent complete runtime

`oni-agent-reviewed-runtime-v1` recognizes only SHA-256
`ca0e6bd1cc4bca341e05059fd5bee97e0ccefec94901a084078997ebc89e3020`,
including its metadata. The reviewed implementation has constructor-only
issuance, allowance-based transfers, metadata-only ownership and one-way
renunciation. It has no runtime mint, tax, trading restriction, recovery,
upgrade, external call or privileged balance operation in its source.
Controls interprets this model; Tax still returns unknown for it.

Provenance: Base mainnet MENTE
`0x66397356a1526f42ef1709de9d3ecbaae76a653b`, block `51669763`, hash
`0x28d50af88e928175b7ba31d5728dad1dbcc55d3dc635dac0f5541c62d3522eb0`
(2026-09-23). Eight sampled tokens had this same complete runtime hash.
[Public source](https://sourcify.dev/server/v2/contract/8453/0x66397356a1526f42ef1709de9d3ecbaae76a653b?fields=compilation,stdJsonInput)
identifies `contracts/OniToken.sol:OniAgent`. [Source](testdata/OniAgent.sol)
and `testdata/oni-agent.json` retain source, native 0.8.20 input/output and the
observed 5306-byte runtime. Compiler SHA-256:
`0479d44fdf9c501c25337fdc540419f1593b884a87b47f023da4f1c700fda782`.

Native recompilation matches the 5253-byte body; the trailing IPFS metadata
digest differs. The generic runtime matcher intentionally rejects this pair
because code-reference opcodes prevent its metadata relaxation. This addition
is an explicitly reviewed, complete runtime entry, not a relaxation of that
matcher or a generic approval of the contract name/source/body hash. Changed
metadata or executable bytes fall through to normal verification and remain
unknown unless independently supported. Production has no research dependency.

In addition to the existing ERC20/Permit and runtime models, `ownable.go` accepts
only the reviewed OpenZeppelin 5.5 ERC20 + Ownable contents and these wrappers:

- A parameterless constructor with literal name/symbol, `Ownable(msg.sender)`,
  and at most one `_mint(msg.sender, numeric_literal)` statement.
- Optionally one public/external, nonpayable `mint(address,uint256)` with exactly
  the inherited `onlyOwner` modifier and the inherited `_mint` call on its inputs.
- No extra storage, overrides, recovery functions, modifiers, assembly or roles.

`testdata/ownable*.json` contains locally generated native Solidity 0.8.37 inputs,
outputs and runtimes for accepted constructors/minting and rejected ownership
recovery/unrestricted mint. These are synthetic contracts, not live deployments.
Tests also reuse the existing Tax fixtures; production never reads testdata or
research code. OpenZeppelin sources retain SPDX notices; see
[license](testdata/OPENZEPPELIN_LICENSE) and the reviewed
[Ownable source](https://github.com/OpenZeppelin/openzeppelin-contracts/blob/v5.5.0/contracts/access/Ownable.sol).

```sh
go test ./evm/erc20/analysis ./evm/erc20/controls ./evm/erc20/tax/...
go test -race ./evm/erc20/analysis ./evm/erc20/controls ./evm/erc20/tax
```
