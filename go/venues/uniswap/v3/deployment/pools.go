package deployment

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
)

type PoolDefinition struct {
	ChainID uint64
	Address common.Address
	Factory common.Address
	Key     protocol.PoolKey
}

// Each entry specifies its actual on-chain token order independently of symbols.
// Base Sepolia USDC/cbBTC pools were verified against factory and pool RPC on
// 2026-09-17. These definitions do not describe liquidity or execution permission.
var poolDefinitions = []PoolDefinition{
	{
		ChainID: 84532,
		Address: common.HexToAddress("0x94bfc0574ff48e92ce43d495376c477b1d0eeec0"),
		Factory: common.HexToAddress("0x4752ba5DBc23f44D87826276BF6Fd6b1C372aD24"),
		Key: protocol.PoolKey{
			Token0: protocol.NewCurrency(common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")),
			Token1: protocol.NewCurrency(common.HexToAddress("0x4200000000000000000000000000000000000006")),
			Fee:    500,
		},
	},
	{
		ChainID: 84532,
		Address: common.HexToAddress("0x46880b404cd35c165eddeff7421019f8dd25f4ad"),
		Factory: common.HexToAddress("0x4752ba5DBc23f44D87826276BF6Fd6b1C372aD24"),
		Key: protocol.PoolKey{
			Token0: protocol.NewCurrency(common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")),
			Token1: protocol.NewCurrency(common.HexToAddress("0x4200000000000000000000000000000000000006")),
			Fee:    3000,
		},
	},
	{
		ChainID: 84532,
		Address: common.HexToAddress("0x859d61699ff73db220608389ce8a71497d66923e"),
		Factory: common.HexToAddress("0x4752ba5DBc23f44D87826276BF6Fd6b1C372aD24"),
		Key: protocol.PoolKey{
			Token0: protocol.NewCurrency(common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")),
			Token1: protocol.NewCurrency(common.HexToAddress("0xcbb7c0006f23900c38eb856149f799620fcb8a4a")),
			Fee:    100,
		},
	},
	{
		ChainID: 84532,
		Address: common.HexToAddress("0x3804f1c3f7abf426111e48fc5f1014c37eeb2c71"),
		Factory: common.HexToAddress("0x4752ba5DBc23f44D87826276BF6Fd6b1C372aD24"),
		Key: protocol.PoolKey{
			Token0: protocol.NewCurrency(common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")),
			Token1: protocol.NewCurrency(common.HexToAddress("0xcbb7c0006f23900c38eb856149f799620fcb8a4a")),
			Fee:    500,
		},
	},
	{
		ChainID: 84532,
		Address: common.HexToAddress("0x56baae1b305a47a500b1d2f784b28ae4cfb66450"),
		Factory: common.HexToAddress("0x4752ba5DBc23f44D87826276BF6Fd6b1C372aD24"),
		Key: protocol.PoolKey{
			Token0: protocol.NewCurrency(common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")),
			Token1: protocol.NewCurrency(common.HexToAddress("0xcbb7c0006f23900c38eb856149f799620fcb8a4a")),
			Fee:    3000,
		},
	},
	{
		ChainID: 84532,
		Address: common.HexToAddress("0x7ce8c98f23e8e994a9bd9c0e4cf8d7ead7fa8546"),
		Factory: common.HexToAddress("0x4752ba5DBc23f44D87826276BF6Fd6b1C372aD24"),
		Key: protocol.PoolKey{
			Token0: protocol.NewCurrency(common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")),
			Token1: protocol.NewCurrency(common.HexToAddress("0xcbb7c0006f23900c38eb856149f799620fcb8a4a")),
			Fee:    10000,
		},
	},
}

// LookupPool returns a copy of a trusted pool definition without RPC access.
// Token order comes from each definition; no symbol or token address is assumed.
//
// Version:
//   - 2026-09-17: Store both tokens explicitly per pool and scope lookup by chain ID.
func LookupPool(chainID uint64, address common.Address) (PoolDefinition, bool) {
	return lookupPool(poolDefinitions, chainID, address)
}

func lookupPool(definitions []PoolDefinition, chainID uint64, address common.Address) (PoolDefinition, bool) {
	for _, definition := range definitions {
		if definition.ChainID == chainID && definition.Address == address {
			return definition, true
		}
	}
	return PoolDefinition{}, false
}
