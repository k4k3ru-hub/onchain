package slipstream

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/crypto"
)

// captureRetainedOracle is initialization-only. All calls are pinned to the
// captured pool block; the producer must verify that block hash after capture.
// Read newest to oldest until the fee window is covered, preserving unknown slots.
func (c *StateCache) captureRetainedOracle(ctx context.Context, s *poolSnapshot, secondsAgo uint32) (retainedOracle, error) {
	if c == nil || c.rpc == nil || s == nil || ctx == nil {
		return retainedOracle{}, fmt.Errorf("failed to capture retained oracle: dependency=null")
	}
	budget := 1
	data, err := c.read(ctx, s, "slot0()", nil, &budget)
	if err != nil {
		return retainedOracle{}, fmt.Errorf("failed to capture retained oracle slot: %w", err)
	}
	slot, err := decodeSlot0(data)
	if err != nil {
		return retainedOracle{}, fmt.Errorf("failed to capture retained oracle slot: %w", err)
	}
	if !slot.Unlocked || slot.SqrtPriceX96.Cmp(s.price) != 0 || slot.Tick != s.tick || slot.ObservationCardinality == 0 || slot.ObservationIndex >= slot.ObservationCardinality || slot.ObservationCardinalityNext < slot.ObservationCardinality {
		return retainedOracle{}, fmt.Errorf("failed to capture retained oracle: slot=mismatch")
	}
	result := retainedOracle{slots: make([]tickObservation, int(slot.ObservationCardinality)), known: make([]bool, int(slot.ObservationCardinality)), index: slot.ObservationIndex, next: slot.ObservationCardinalityNext}
	for start := 0; start < len(result.slots); start += 16 {
		count := min(16, len(result.slots)-start)
		calls := make([][]byte, count)
		for i := range calls {
			index := (int(result.index) - start - i + len(result.slots)) % len(result.slots)
			calls[i] = append(append([]byte(nil), crypto.Keccak256([]byte("observations(uint256)"))[:4]...), new(big.Int).SetUint64(uint64(index)).FillBytes(make([]byte, 32))...)
		}
		var outputs [][]byte
		if batch, ok := c.rpc.(batchStateReader); ok {
			values, failures, err := batch.ReadContracts(ctx, c.pool, calls, s.header.Number)
			if err != nil {
				return retainedOracle{}, fmt.Errorf("failed to capture retained oracle batch: %w", err)
			}
			if len(values) != count || len(failures) != count {
				return retainedOracle{}, fmt.Errorf("failed to capture retained oracle batch: results=invalid")
			}
			if err := errors.Join(failures...); err != nil {
				return retainedOracle{}, fmt.Errorf("failed to capture retained oracle batch: %w", err)
			}
			outputs = values
		} else {
			outputs = make([][]byte, count)
			for i, call := range calls {
				outputs[i], err = c.rpc.CallContract(ctx, ethereum.CallMsg{To: &c.pool, Data: call}, new(big.Int).SetUint64(s.header.Number))
				if err != nil {
					return retainedOracle{}, fmt.Errorf("failed to capture retained oracle observation: %w", err)
				}
			}
		}
		for i, value := range outputs {
			index := (int(result.index) - start - i + len(result.slots)) % len(result.slots)
			observation, err := decodeTickObservation(value)
			if err != nil {
				return retainedOracle{}, fmt.Errorf("failed to capture retained oracle observation: %w: index=%d", err, index)
			}
			result.slots[index] = observation
			result.known[index] = true
		}
		// A loaded observation at or before the target closes the required window.
		for i := 0; i < count; i++ {
			index := (int(result.index) - start - i + len(result.slots)) % len(result.slots)
			observation := result.slots[index]
			if observation.initialized && uint32(s.header.Timestamp)-observation.timestamp >= secondsAgo {
				if err := result.validate(); err != nil {
					return retainedOracle{}, fmt.Errorf("failed to capture retained oracle: %w", err)
				}
				return result, nil
			}
		}
	}
	if err := result.validate(); err != nil {
		return retainedOracle{}, fmt.Errorf("failed to capture retained oracle: %w", err)
	}
	return result, nil
}

func decodeTickObservation(data []byte) (tickObservation, error) {
	if len(data) != 128 {
		return tickObservation{}, fmt.Errorf("failed to decode retained observation: data=invalid")
	}
	timestamp, err := decodeUnsignedWord(data[:32], 32, "timestamp")
	if err != nil {
		return tickObservation{}, err
	}
	cumulative := new(big.Int).SetBytes(data[32:64])
	if cumulative.Bit(255) != 0 {
		cumulative.Sub(cumulative, power2(256))
	}
	if !cumulative.IsInt64() || cumulative.Int64() < -(1<<55) || cumulative.Int64() >= 1<<55 {
		return tickObservation{}, fmt.Errorf("failed to decode retained observation: cumulative=out_of_range")
	}
	if _, err := decodeUnsignedWord(data[64:96], 160, "seconds_per_liquidity"); err != nil {
		return tickObservation{}, err
	}
	initialized, err := decodeUnsignedWord(data[96:], 1, "initialized")
	if err != nil {
		return tickObservation{}, err
	}
	return tickObservation{timestamp: uint32(timestamp.Uint64()), cumulative: cumulative.Int64(), initialized: initialized.Sign() != 0}, nil
}
