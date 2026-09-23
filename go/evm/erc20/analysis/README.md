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
