package evm

import "github.com/ethereum/go-ethereum/common"

type tokenDefinitionKey struct {
	chainID ChainID
	address common.Address
}

// Definitions cover configured MarketHub tokens whose metadata was observed
// during initialization on 2026-09-16. They are trusted reference data, not
// evidence of historical contract state or token safety.
var tokenDefinitions = map[tokenDefinitionKey]TokenMetadata{
	{ChainIDEthereumMainnet, common.HexToAddress("0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599")}:  {Symbol: "WBTC", Address: "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", Decimals: 8},
	{ChainIDEthereumMainnet, common.HexToAddress("0xdAC17F958D2ee523a2206206994597C13D831ec7")}:  {Symbol: "USDT", Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6},
	{ChainIDBaseMainnet, common.HexToAddress("0x4200000000000000000000000000000000000006")}:      {Symbol: "WETH", Address: "0x4200000000000000000000000000000000000006", Decimals: 18},
	{ChainIDBaseMainnet, common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")}:      {Symbol: "USDC", Address: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Decimals: 6},
	{ChainIDBaseSepolia, common.HexToAddress("0x4200000000000000000000000000000000000006")}:      {Symbol: "WETH", Address: "0x4200000000000000000000000000000000000006", Decimals: 18},
	{ChainIDBaseSepolia, common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")}:      {Symbol: "USDC", Address: "0x036CbD53842c5426634e7929541eC2318f3dCF7e", Decimals: 6},
	{ChainIDRobinhoodMainnet, common.HexToAddress("0x39dBED3a2bd333467115dE45665cC57F813C4571")}: {Symbol: "PONS", Address: "0x39dBED3a2bd333467115dE45665cC57F813C4571", Decimals: 18},
	{ChainIDRobinhoodMainnet, common.HexToAddress("0x5fc5360D0400a0Fd4f2af552ADD042D716F1d168")}: {Symbol: "USDG", Address: "0x5fc5360D0400a0Fd4f2af552ADD042D716F1d168", Decimals: 6},
}

// LookupTokenMetadata returns trusted reference metadata for a chain and address.
// It performs no RPC and returns false for undefined tokens, including native currency.
// Returned values are copies; historical reads must not use these definitions.
//
// Version:
//   - 2026-09-17: Added.
func LookupTokenMetadata(chainID ChainID, address common.Address) (TokenMetadata, bool) {
	value, ok := tokenDefinitions[tokenDefinitionKey{chainID, address}]
	return value, ok
}
