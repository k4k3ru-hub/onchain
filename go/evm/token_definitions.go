package evm

import "github.com/ethereum/go-ethereum/common"

type tokenDefinitionKey struct {
	chainID ChainID
	address common.Address
}

// ERC20 definitions cover configured MarketHub tokens whose metadata was observed
// during initialization on 2026-09-16. They are trusted reference data, not
// evidence of historical contract state or token safety.
// Base Sepolia cbBTC was additionally verified by RPC on 2026-09-17; its address
// is documented at https://docs.horizen.io/horizen-chain/tokens-and-gas/cbtc/.
// Native entries are explicit current chain definitions, with the zero address
// as the native-currency identifier; they are not ERC20 contract observations.
// ERC20 names are optional current reference data; see TOKEN_METADATA.md for provenance.
var tokenDefinitions = map[tokenDefinitionKey]TokenMetadata{
	{ChainIDEthereumMainnet, common.Address{}}:                                                   {Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18},
	{ChainIDEthereumSepolia, common.Address{}}:                                                   {Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18},
	{ChainIDBaseMainnet, common.Address{}}:                                                       {Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18},
	{ChainIDBaseSepolia, common.Address{}}:                                                       {Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18},
	{ChainIDRobinhoodMainnet, common.Address{}}:                                                  {Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18},
	{ChainIDRobinhoodTestnet, common.Address{}}:                                                  {Symbol: "ETH", Address: "0x0000000000000000000000000000000000000000", Decimals: 18},
	{ChainIDBNBMainnet, common.Address{}}:                                                        {Symbol: "BNB", Address: "0x0000000000000000000000000000000000000000", Decimals: 18},
	{ChainIDPolygonMainnet, common.Address{}}:                                                    {Symbol: "POL", Address: "0x0000000000000000000000000000000000000000", Decimals: 18},
	{ChainIDPolygonAmoy, common.Address{}}:                                                       {Symbol: "POL", Address: "0x0000000000000000000000000000000000000000", Decimals: 18},
	{ChainIDBaseSepolia, common.HexToAddress("0xcbb7c0006f23900c38eb856149f799620fcb8a4a")}:      {Symbol: "cbBTC", Name: "Coinbase Wrapped BTC", Address: "0xcbb7c0006f23900c38eb856149f799620fcb8a4a", Decimals: 8},
	{ChainIDEthereumMainnet, common.HexToAddress("0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599")}:  {Symbol: "WBTC", Name: "Wrapped BTC", Address: "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", Decimals: 8},
	{ChainIDEthereumMainnet, common.HexToAddress("0xdAC17F958D2ee523a2206206994597C13D831ec7")}:  {Symbol: "USDT", Name: "Tether USD", Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6},
	{ChainIDBaseMainnet, common.HexToAddress("0x4200000000000000000000000000000000000006")}:      {Symbol: "WETH", Name: "Wrapped Ether", Address: "0x4200000000000000000000000000000000000006", Decimals: 18},
	{ChainIDBaseMainnet, common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")}:      {Symbol: "USDC", Name: "USD Coin", Address: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Decimals: 6},
	{ChainIDBaseSepolia, common.HexToAddress("0x4200000000000000000000000000000000000006")}:      {Symbol: "WETH", Name: "Wrapped Ether", Address: "0x4200000000000000000000000000000000000006", Decimals: 18},
	{ChainIDBaseSepolia, common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")}:      {Symbol: "USDC", Name: "USDC", Address: "0x036CbD53842c5426634e7929541eC2318f3dCF7e", Decimals: 6},
	{ChainIDRobinhoodMainnet, common.HexToAddress("0x39dBED3a2bd333467115dE45665cC57F813C4571")}: {Symbol: "PONS", Name: "Pons", Address: "0x39dBED3a2bd333467115dE45665cC57F813C4571", Decimals: 18},
	{ChainIDRobinhoodMainnet, common.HexToAddress("0x5fc5360D0400a0Fd4f2af552ADD042D716F1d168")}: {Symbol: "USDG", Name: "Global Dollar", Address: "0x5fc5360D0400a0Fd4f2af552ADD042D716F1d168", Decimals: 6},
}

// LookupTokenMetadata returns trusted reference metadata for a chain and address.
// The zero address identifies native currency on explicitly registered chains.
// It performs no RPC and returns false for undefined chain/address combinations.
// Returned values are copies of current reference definitions, not historical RPC
// observations. Historical ERC20 reads must query the contract at the requested block.
//
// Version:
//   - 2026-09-17: Added.
//   - 2026-09-20: Resolve native currency metadata by chain ID and zero address.
//   - 2026-09-23: Include verified ERC20 reference names when available.
func LookupTokenMetadata(chainID ChainID, address common.Address) (TokenMetadata, bool) {
	value, ok := tokenDefinitions[tokenDefinitionKey{chainID, address}]
	return value, ok
}
