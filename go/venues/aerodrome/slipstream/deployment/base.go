package deployment

import "github.com/ethereum/go-ethereum/common"

const baseMainnetChainID uint64 = 8453

func baseMainnetLegacy() Deployment {
	return Deployment{
		ID:       IDAerodromeBaseMainnetLegacy,
		ChainID:  baseMainnetChainID,
		Venue:    VenueAerodrome,
		Factory:  common.HexToAddress("0x5e7BB104d84c7CB9B682AaC2F3d509f5F406809A"),
		Router:   common.HexToAddress("0xBE6D8f0d05cC4be24d5167a3eF062215bE6D18a5"),
		QuoterV2: common.HexToAddress("0x254cF9E1E6e233aa1AC962CB9B05b2cfeAaE15b0"),
	}
}

func baseMainnetCurrent() Deployment {
	return Deployment{
		ID:       IDAerodromeBaseMainnetCurrent,
		ChainID:  baseMainnetChainID,
		Venue:    VenueAerodrome,
		Factory:  common.HexToAddress("0xf8f2eB4940CFE7d13603DDDD87f123820Fc061Ef"),
		Router:   common.HexToAddress("0xBE6D8f0d05cC4be24d5167a3eF062215bE6D18a5"),
		QuoterV2: common.HexToAddress("0x514c8B5f54112481E28028F1166Bd78501089259"),
	}
}
