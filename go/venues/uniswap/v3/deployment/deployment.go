package deployment

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

var poolInitCodeHash = common.HexToHash("0xe34f199b19b2b4f47f68442619d555527d244f78a3297ea89325f843f87b8b54")

type Deployment struct {
	ChainID      uint64
	Factory      common.Address
	SwapRouter02 common.Address
	QuoterV2     common.Address
	InitCodeHash common.Hash
}

// ByChainID returns the supported official Uniswap v3 deployment for a chain.
//
// Parameters:
//   - chainID: EVM chain ID.
//
// Returns:
//   - Official Uniswap v3 deployment.
//   - Lookup error for an unsupported chain.
//
// Version:
//   - 2026-09-06: Added Robinhood Chain Mainnet.
//   - 2026-08-24: Added.
func ByChainID(chainID uint64) (Deployment, error) {
	var factory, swapRouter02, quoterV2 string
	switch chainID {
	case 1:
		factory = "0x1F98431c8aD98523631AE4a59f267346ea31F984"
		swapRouter02 = "0x68b3465833fb72A70ecDF485E0e4C7bD8665Fc45"
		quoterV2 = "0x61fFE014bA17989E743c5F6cB21bF9697530B21e"
	case 56:
		factory = "0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7"
		swapRouter02 = "0xB971eF87ede563556b2ED4b1C0b0019111Dd85d2"
		quoterV2 = "0x78D78E420Da98ad378D7799bE8f4AF69033EB077"
	case 8453:
		factory = "0x33128a8fC17869897dcE68Ed026d694621f6FDfD"
		swapRouter02 = "0x2626664c2603336E57B271c5C0b26F421741e481"
		quoterV2 = "0x3d4e44Eb1374240CE5F1B871ab261CD16335B76a"
	case 84532:
		factory = "0x4752ba5DBc23f44D87826276BF6Fd6b1C372aD24"
		swapRouter02 = "0x94cC0AaC535CCDB3C01d6787D6413C739ae12bc4"
		quoterV2 = "0xC5290058841028F1614F3A6F0F5816cAd0df5E27"
	case 4663:
		factory = "0x1f7d7550B1b028f7571E69A784071F0205FD2EfA"
		quoterV2 = "0x33e885eD0Ec9bF04EcfB19341582aADCb4c8A9E7"
	default:
		return Deployment{}, fmt.Errorf("failed to resolve uniswap v3 deployment: official contracts are unavailable: chain_id=%d", chainID)
	}
	return Deployment{
		ChainID:      chainID,
		Factory:      common.HexToAddress(factory),
		SwapRouter02: common.HexToAddress(swapRouter02),
		QuoterV2:     common.HexToAddress(quoterV2),
		InitCodeHash: poolInitCodeHash,
	}, nil
}
