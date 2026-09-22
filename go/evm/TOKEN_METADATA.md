# Reference token names

`TokenMetadata.Name` is optional trusted reference data. `LookupTokenMetadata`
matches chain ID and address, returns a copy, and performs no RPC. An empty name
means no reference name is supplied; it must not be replaced with the symbol.
Names are current configuration, not proof of a name at an arbitrary historical
block. Existing native definitions leave the name empty because they are not
ERC20 contract-name observations.

The following ERC20 names were confirmed on 2026-09-23 JST. Existing MarketHub
snapshots with `name.status = observed` were reused for six definitions. The three
remaining names were read using `name()` at a fixed block after checking chain ID.

| Chain / network | Address | Name | Evidence |
| --- | --- | --- | --- |
| Ethereum mainnet | `0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599` | Wrapped BTC | Stored observation |
| Ethereum mainnet | `0xdAC17F958D2ee523a2206206994597C13D831ec7` | Tether USD | Stored observation |
| Base mainnet | `0x4200000000000000000000000000000000000006` | Wrapped Ether | Stored observation |
| Base mainnet | `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` | USD Coin | Stored observation |
| Base Sepolia | `0x4200000000000000000000000000000000000006` | Wrapped Ether | Stored observation |
| Base Sepolia | `0x036CbD53842c5426634e7929541eC2318f3dCF7e` | USDC | RPC, block `0x2cfa769` |
| Base Sepolia | `0xcbb7c0006f23900c38eb856149f799620fcb8a4a` | Coinbase Wrapped BTC | RPC, block `0x2cfa769` |
| Robinhood mainnet | `0x39dBED3a2bd333467115dE45665cC57F813C4571` | Pons | RPC, block `0x429131c` |
| Robinhood mainnet | `0x5fc5360D0400a0Fd4f2af552ADD042D716F1d168` | Global Dollar | Stored observation |

Base Sepolia USDC returns `USDC` as its name, unlike Base mainnet's `USD Coin`.
Do not copy names between networks based on a shared symbol. Adding a new name
requires a chain/address-specific observation; runtime users may choose to trust
the definition and omit the contract read.
