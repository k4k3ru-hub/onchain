package slipstream

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type retainedFeeState struct {
	module common.Address
	config dynamicFeeInputs
	oracle retainedOracle
}

// captureRetainedFeeState admits a baseline only after local fee parity and
// final block-hash verification. Failed captures never publish partial state.
func (c *StateCache) captureRetainedFeeState(ctx context.Context, s *poolSnapshot) (retainedFeeState, error) {
	module, config, err := c.captureRetainedFee(ctx, s)
	if err != nil {
		return retainedFeeState{}, err
	}
	oracle, err := c.captureRetainedOracle(ctx, s)
	if err != nil {
		return retainedFeeState{}, err
	}
	fee, err := oracle.fee(config, uint32(s.header.Timestamp), s.tick)
	if err != nil {
		return retainedFeeState{}, fmt.Errorf("failed to verify retained fee: %w", err)
	}
	if fee != s.fee {
		return retainedFeeState{}, fmt.Errorf("failed to verify retained fee: fee=mismatch local_fee=%d pool_fee=%d", fee, s.fee)
	}
	header, err := c.rpc.HeaderByNumber(ctx, s.header.Number)
	if err != nil {
		return retainedFeeState{}, fmt.Errorf("failed to verify retained fee block: %w", err)
	}
	if s.header.Hash == (common.Hash{}) || header.Number != s.header.Number || header.Hash != s.header.Hash {
		return retainedFeeState{}, fmt.Errorf("failed to verify retained fee block: block=mismatch")
	}
	return retainedFeeState{module: module, config: config, oracle: oracle}, nil
}

// captureRetainedFee reads the supported five-field DynamicSwapFeeModule ABI.
// It is initialization-only. The producer must subscribe to the returned module
// before accepting this capture and verify the baseline block hash afterwards.
// Unknown modules and failed reads must not silently become a fixed fee.
func (c *StateCache) captureRetainedFee(ctx context.Context, s *poolSnapshot) (common.Address, dynamicFeeInputs, error) {
	var empty dynamicFeeInputs
	if c == nil || c.rpc == nil || s == nil || ctx == nil {
		return common.Address{}, empty, fmt.Errorf("failed to capture retained fee: dependency=null")
	}
	read := func(address common.Address, signature string, argument *big.Int) ([]byte, error) {
		data := append([]byte(nil), crypto.Keccak256([]byte(signature))[:4]...)
		if argument != nil {
			data = append(data, argument.FillBytes(make([]byte, 32))...)
		}
		value, err := c.rpc.CallContract(ctx, ethereum.CallMsg{To: &address, Data: data}, new(big.Int).SetUint64(s.header.Number))
		if err != nil {
			return nil, fmt.Errorf("failed to capture retained fee: %w: method=%q", err, signature)
		}
		return value, nil
	}
	data, err := read(c.factory, "swapFeeModule()", nil)
	if err != nil {
		return common.Address{}, empty, err
	}
	moduleWord, err := decodeUnsignedWord(data, 160, "fee_module")
	if err != nil {
		return common.Address{}, empty, fmt.Errorf("failed to capture retained fee module: %w", err)
	}
	module := common.BigToAddress(moduleWord)
	if module == (common.Address{}) {
		return module, empty, fmt.Errorf("failed to capture retained fee: fee_module=unsupported")
	}
	data, err = read(module, "factory()", nil)
	if err != nil {
		return module, empty, err
	}
	factory, err := decodeUnsignedWord(data, 160, "factory")
	if err != nil {
		return module, empty, fmt.Errorf("failed to capture retained fee factory: %w", err)
	}
	if common.BigToAddress(factory) != c.factory {
		return module, empty, fmt.Errorf("failed to capture retained fee: factory=mismatch")
	}
	data, err = read(module, "dynamicFeeConfig(address)", new(big.Int).SetBytes(c.pool[:]))
	if err != nil {
		return module, empty, err
	}
	config, err := decodeRetainedFeeConfig(data)
	if err != nil {
		return module, empty, err
	}
	fields := []struct {
		address common.Address
		method  string
		arg     *big.Int
		max     uint64
		set     func(uint64)
	}{
		{c.factory, "tickSpacingToFee(int24)", big.NewInt(int64(s.spacing)), 100000, func(v uint64) { config.spacingFee = uint32(v) }},
		{module, "defaultScalingFactor()", nil, 1e18, func(v uint64) { config.defaultScaling = v }},
		{module, "defaultFeeCap()", nil, 50000, func(v uint64) { config.defaultCap = uint32(v) }},
		{module, "secondsAgo()", nil, 131069, func(v uint64) { config.secondsAgo = uint32(v) }},
		// Existing pool fee() eth_calls use the zero origin, independent of the Trade sender.
		{module, "discounted(address)", new(big.Int), 500000, func(v uint64) { config.discount = uint32(v) }},
	}
	if s.spacing <= 0 || s.spacing > 16383 {
		return module, empty, fmt.Errorf("failed to capture retained fee: tick_spacing=out_of_range")
	}
	for _, field := range fields {
		data, err := read(field.address, field.method, field.arg)
		if err != nil {
			return module, empty, err
		}
		n, err := decodeUnsignedWord(data, 64, "fee_configuration")
		if err != nil {
			return module, empty, fmt.Errorf("failed to capture retained fee setting: %w: method=%q", err, field.method)
		}
		if n.Uint64() > field.max {
			return module, empty, fmt.Errorf("failed to capture retained fee: value=out_of_range method=%q", field.method)
		}
		field.set(n.Uint64())
	}
	if config.secondsAgo < 2 {
		return module, empty, fmt.Errorf("failed to capture retained fee: seconds_ago=out_of_range")
	}
	return module, config, nil
}

func decodeRetainedFeeConfig(data []byte) (dynamicFeeInputs, error) {
	var empty dynamicFeeInputs
	if len(data) != 160 {
		return empty, fmt.Errorf("failed to decode retained fee: configuration=unsupported")
	}
	values := make([]uint64, 5)
	limits := []uint64{30000, 50000, 1e18, 1, 50000}
	for i, limit := range limits {
		n, err := decodeUnsignedWord(data[i*32:(i+1)*32], 64, "fee_configuration")
		if err != nil {
			return empty, fmt.Errorf("failed to decode retained fee: %w", err)
		}
		if n.Uint64() > limit {
			return empty, fmt.Errorf("failed to decode retained fee: value=out_of_range field=%d", i)
		}
		values[i] = n.Uint64()
	}
	return dynamicFeeInputs{base: uint32(values[0]), cap: uint32(values[1]), scaling: values[2], initialEnabled: values[3] == 1, initial: uint32(values[4])}, nil
}
