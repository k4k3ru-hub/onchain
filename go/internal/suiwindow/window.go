package suiwindow

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

var ErrCoverage = errors.New("coverage=insufficient")

const Radius = 1
const batchSize = 32
const maxTick = 443636

type Reader interface {
	DynamicValuesByKeysAtCheckpoint(context.Context, sui.Address, sui.CheckpointSequenceNumber, []sui.DynamicFieldKey) ([]json.RawMessage, error)
}

// Word returns the bitmap word containing a tick, using floor division.
//
// Version:
//   - 2026-09-11: Added.
func Word(tick, spacing int32) int32 {
	compressed := tick / spacing
	if tick < 0 && tick%spacing != 0 {
		compressed--
	}
	return compressed >> 8
}

// Missing checks every initialized tick in the current three-word window without IO.
//
// Version:
//   - 2026-09-11: Added.
func Missing(tick, spacing int32, words, nets map[int32]*big.Int) bool {
	center := Word(tick, spacing)
	for word := center - Radius; word <= center+Radius; word++ {
		if word < Word(-maxTick, spacing) || word > Word(maxTick, spacing) {
			continue
		}
		bits := words[word]
		if bits == nil {
			return true
		}
		for bit := 0; bit < 256; bit++ {
			index := (word*256 + int32(bit)) * spacing
			if index >= -maxTick && index <= maxTick && bits.Bit(bit) != 0 && nets[index] == nil {
				return true
			}
		}
	}
	return false
}

// Read acquires nearby bitmap words and every initialized tick at one checkpoint.
// Results are staged until all chunks succeed; failures never fall back to individual requests.
//
// Version:
//   - 2026-09-11: Added.
func Read(ctx context.Context, reader Reader, checkpoint sui.CheckpointSequenceNumber, bitmap, ticks sui.Address, keyType string, tick, spacing int32, unsigned func(json.RawMessage) (*big.Int, error)) (map[int32]*big.Int, map[int32]*big.Int, error) {
	if ctx == nil || reader == nil || unsigned == nil || spacing <= 0 || spacing > maxTick || tick < -maxTick || tick > maxTick {
		return nil, nil, fmt.Errorf("failed to capture sui tick window: parameters=invalid")
	}
	read := func(parent sui.Address, indexes []int32) ([]json.RawMessage, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		keys := make([]sui.DynamicFieldKey, len(indexes))
		for i, index := range indexes {
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, uint32(index))
			keys[i] = sui.DynamicFieldKey{Type: keyType, BCS: b}
		}
		values, err := reader.DynamicValuesByKeysAtCheckpoint(ctx, parent, checkpoint, keys)
		if err != nil {
			return nil, err
		}
		if len(values) != len(keys) {
			return nil, fmt.Errorf("failed to capture sui tick window: values=invalid")
		}
		return values, nil
	}
	center := Word(tick, spacing)
	var indexes []int32
	for word := center - Radius; word <= center+Radius; word++ {
		if word >= Word(-maxTick, spacing) && word <= Word(maxTick, spacing) {
			indexes = append(indexes, word)
		}
	}
	values, err := read(bitmap, indexes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to capture sui bitmap window: %w", err)
	}
	words, nets := make(map[int32]*big.Int), make(map[int32]*big.Int)
	var initialized []int32
	for i, word := range indexes {
		bits := new(big.Int)
		if len(values[i]) != 0 {
			bits, err = unsigned(values[i])
			if err != nil {
				return nil, nil, fmt.Errorf("failed to decode sui bitmap window: %w", err)
			}
		}
		if bits == nil || bits.Sign() < 0 || bits.BitLen() > 256 {
			return nil, nil, fmt.Errorf("failed to decode sui bitmap window: word=out_of_range")
		}
		words[word] = bits
		for bit := 0; bit < 256; bit++ {
			index := (word*256 + int32(bit)) * spacing
			if bits.Bit(bit) != 0 && index >= -maxTick && index <= maxTick {
				initialized = append(initialized, index)
			}
		}
	}
	for start := 0; start < len(initialized); start += batchSize {
		chunk := initialized[start:min(start+batchSize, len(initialized))]
		values, err := read(ticks, chunk)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to capture sui window ticks: %w", err)
		}
		for i, raw := range values {
			var value struct {
				Net struct {
					Bits json.RawMessage `json:"bits"`
				} `json:"liquidity_net"`
			}
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, nil, fmt.Errorf("failed to decode sui window tick: %w: tick=%d", err, chunk[i])
			}
			net, err := unsigned(value.Net.Bits)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to decode sui window tick: %w: tick=%d", err, chunk[i])
			}
			if net == nil || net.Sign() < 0 || net.BitLen() > 128 {
				return nil, nil, fmt.Errorf("failed to decode sui window tick: liquidity=out_of_range tick=%d", chunk[i])
			}
			if net.Bit(127) != 0 {
				net.Sub(net, new(big.Int).Lsh(big.NewInt(1), 128))
			}
			nets[chunk[i]] = net
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to capture sui tick window: %w", err)
	}
	return words, nets, nil
}
