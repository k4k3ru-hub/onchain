package slipstream

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm/poolfee"
)

// PoolFeeModules returns independently allocated, published fee module addresses.
// These reviewed deployments are documented in aerodrome-finance/slipstream's
// deployment list. New modules require explicit review before being added here.
//
// Version:
//   - 2026-09-22: Added.
func PoolFeeModules(chainID uint64, factory common.Address) []common.Address {
	if chainID != 8453 {
		return nil
	}
	var values []string
	switch factory {
	case common.HexToAddress("0x5e7BB104d84c7CB9B682AaC2F3d509f5F406809A"):
		values = []string{"0xF4171B0953b52Fa55462E4d76ecA1845Db69af00", "0x090b2A6bb475c00e2256e2095A60887cD710803b"}
	case common.HexToAddress("0xaDe65c38CD4849aDBA595a4323a8C7DdfE89716a"):
		values = []string{"0x5264Eeeab16037A7A7AF15Ff69A470af6e2a2223", "0xF4Ecd78EBEB6d36CF7f80B5B6B41453515fe2785"}
	case common.HexToAddress("0xf8f2eB4940CFE7d13603DDDD87f123820Fc061Ef"):
		values = []string{"0x87D8f999BBa9343E8099552426775B51C338E8CB"}
	}
	result := make([]common.Address, len(values))
	for i, v := range values {
		result[i] = common.HexToAddress(v)
	}
	return result
}

// ReadPoolFees reads the effective Pool fee for one block and reference sender.
// It verifies factory bindings and supported modules, then invokes fee() with no
// tick or oracle-history downloads. Both swap directions use this Pool getter.
// Canonical block verification and retries belong to the caller.
//
// Version:
//   - 2026-09-22: Added.
func ReadPoolFees(ctx context.Context, rpc poolfee.ContractReader, chainID uint64, factory, pool, sender common.Address, block *big.Int) (poolfee.Rates, error) {
	modules := PoolFeeModules(chainID, factory)
	if len(modules) == 0 {
		return poolfee.Rates{}, fmt.Errorf("failed to read slipstream fees: %w", poolfee.ErrUnsupported)
	}
	readAddress := func(address common.Address, signature string) (common.Address, error) {
		raw, err := poolfee.Call(ctx, rpc, address, sender, block, signature, nil)
		if err != nil {
			return common.Address{}, err
		}
		return poolfee.AddressWord(raw)
	}
	bound, err := readAddress(pool, "factory()")
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read slipstream fees: %w", err)
	}
	if bound != factory {
		return poolfee.Rates{}, fmt.Errorf("failed to read slipstream fees: factory=mismatch")
	}
	module, err := readAddress(factory, "swapFeeModule()")
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read slipstream fees: %w", err)
	}
	supported := false
	for _, address := range modules {
		supported = supported || address == module
	}
	if !supported {
		return poolfee.Rates{}, fmt.Errorf("failed to read slipstream fees: %w", poolfee.ErrUnsupported)
	}
	bound, err = readAddress(module, "factory()")
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read slipstream fees: %w", err)
	}
	if bound != factory {
		return poolfee.Rates{}, fmt.Errorf("failed to read slipstream fees: module_factory=mismatch")
	}
	raw, err := poolfee.Call(ctx, rpc, pool, sender, block, "fee()", nil)
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read slipstream fees: %w", err)
	}
	value, err := poolfee.UintWord(raw, 100000)
	if err != nil {
		return poolfee.Rates{}, fmt.Errorf("failed to read slipstream fees: %w", err)
	}
	return poolfee.Rates{Token0ToToken1: uint32(value), Token1ToToken0: uint32(value)}, nil
}
