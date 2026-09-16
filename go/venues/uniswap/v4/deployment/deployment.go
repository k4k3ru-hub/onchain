package deployment

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

type Deployment struct {
	ChainID     uint64
	PoolManager common.Address
	StateView   common.Address
	Quoter      common.Address
}

// ByChainID returns the configured Uniswap v4 deployment for a chain.
//
// Parameters:
//   - chainID: EVM chain ID.
//
// Returns:
//   - Official deployment.
//   - Lookup error when Uniswap does not publish a PoolManager for the chain.
//
// Version:
//   - 2026-09-16: Move deployment lookup into onchain without changing addresses.
//   - 2026-08-19: Included the verified V4Quoter when available.
//   - 2026-09-06: Included the Robinhood Chain Mainnet V4Quoter.
//   - 2026-08-17: Added.
func ByChainID(chainID uint64) (Deployment, error) {
	poolManager, stateView, ok := contractAddresses(chainID)
	if !ok {
		return Deployment{}, fmt.Errorf(
			"failed to resolve uniswap v4 deployment: official contracts are unavailable: chain_id=%d",
			chainID,
		)
	}

	quoter := quoterAddress(chainID)
	return Deployment{
		ChainID:     chainID,
		PoolManager: common.HexToAddress(poolManager),
		StateView:   common.HexToAddress(stateView),
		Quoter:      common.HexToAddress(quoter),
	}, nil
}

// QuoterByChainID returns the configured Uniswap v4 Quoter for a chain.
//
// Parameters:
//   - chainID: EVM chain ID.
//
// Returns:
//   - Official V4Quoter address.
//   - Lookup error when a verified V4Quoter is unavailable for the chain.
//
// Version:
//   - 2026-09-16: Move deployment lookup into onchain without changing addresses.
//   - 2026-08-19: Added.
//   - 2026-09-06: Added Robinhood Chain Mainnet.
func QuoterByChainID(chainID uint64) (common.Address, error) {
	address := quoterAddress(chainID)
	if address == "" {
		return common.Address{}, fmt.Errorf("failed to resolve uniswap v4 quoter deployment: official contract is unavailable: chain_id=%d", chainID)
	}
	return common.HexToAddress(address), nil
}

func quoterAddress(chainID uint64) string {
	switch chainID {
	case 1:
		return "0x52f0e24d1c21c8a0cb1e5a5dd6198556bd9e1203"
	case 56:
		return "0x9f75dd27d6664c475b90e105573e550ff69437b0"
	case 8453:
		return "0x0d5e0f971ed27fbff6c2837bf31316121532048d"
	case 4663:
		return "0x8dc178efb8111bb0973dd9d722ebeff267c98f94"
	default:
		return ""
	}
}

func contractAddresses(chainID uint64) (string, string, bool) {
	switch chainID {
	case 1:
		return "0x000000000004444c5dc75cB358380D2e3dE08A90", "0x7ffe42c4a5deea5b0fec41c94c136cf115597227", true
	case 10:
		return "0x9a13f98cb987694c9f086b1f5eb990eea8264ec3", "0xc18a3169788f4f75a170290584eca6395c75ecdb", true
	case 56:
		return "0x28e2ea090877bf75740558f6bfb36a5ffee9e9df", "0xd13dd3d6e93f276fafc9db9e6bb47c1180aee0c4", true
	case 130:
		return "0x1F98400000000000000000000000000000000004", "0x86e8631a016f9068c3f085faf484ee3f5fdee8f2", true
	case 137:
		return "0x67366782805870060151383f4bbff9dab53e5cd6", "0x5ea1bd7974c8a611cbab0bdcafcb1d9cc9b3ba5a", true
	case 143:
		return "0x188d586ddcf52439676ca21a244753fa19f9ea8e", "0x77395f3b2e73ae90843717371294fa97cc419d64", true
	case 196:
		return "0x360e68faccca8ca495c1b759fd9eee466db9fb32", "0x76fd297e2d437cd7f76d50f01afe6160f86e9990", true
	case 480:
		return "0xb1860d529182ac3bc1f51fa2abd56662b7d13f33", "0x51d394718bc09297262e368c1a481217fdeb71eb", true
	case 1301:
		return "0x9cb26a7183b2f4515945dc52cb4195b0d2d06c95", "0x792d13207744f132943cdde4d37ec89f20ae3b0d", true
	case 1868:
		return "0x360e68faccca8ca495c1b759fd9eee466db9fb32", "0x76fd297e2d437cd7f76d50f01afe6160f86e9990", true
	case 4217:
		return "0x33620f62c5b9b2086dd6b62f4a297a9f30347029", "0x21b954fba3f5ddebe77ef2d47a3100c066908b2a", true
	case 4326:
		return "0xacb7e78fa05d562e0a5d3089ec896d57d057d38e", "0x726f84e1dfb8d375a365e0808282f40d52d3e4e8", true
	case 4663, 5042:
		return "0x8366a39cc670b4001a1121b8f6a443a643e40951", "0xf3334192d15450cdd385c8b70e03f9a6bd9e673b", true
	case 8453:
		return "0x498581ff718922c3f8e6a244956af099b2652b2b", "0xa3c0c9b65bad0b08107aa264b0f3db444b867a71", true
	case 42161:
		return "0x360e68faccca8ca495c1b759fd9eee466db9fb32", "0x76fd297e2d437cd7f76d50f01afe6160f86e9990", true
	case 42220:
		return "0x288dc841A52FCA2707c6947B3A777c5E56cd87BC", "0xbc21f8720babf4b20d195ee5c6e99c52b76f2bfb", true
	case 57073:
		return "0x360e68faccca8ca495c1b759fd9eee466db9fb32", "0x76fd297e2d437cd7f76d50f01afe6160f86e9990", true
	case 59144:
		return "0x248083fb965359d82b06c1f5322480dcfc1ad857", "0xe861de206e460a8b936b05ad3816520b58ccdf9b", true
	case 81457:
		return "0x1631559198a9e474033433b2958dabc135ab6446", "0x12a88ae16f46dce4e8b15368008ab3380885df30", true
	case 11155111:
		return "0xE03A1074c86CFeDd5C142C4F04F1a1536e203543", "0xE1Dd9c3fA50EDB962E442f60DfBc432e24537E4C", true
	case 7777777:
		return "0x0575338e4c17006ae181b47900a84404247ca30f", "0x385785af07d63b50d0a0ea57c4ff89d06adf7328", true
	default:
		return "", "", false
	}
}
