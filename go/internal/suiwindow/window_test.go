package suiwindow

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/sui"
	"math/big"
	"testing"
)

type windowReader struct {
	calls   []int
	failAt  int
	failure error
	cancel  context.CancelFunc
}

// DynamicValuesByKeysAtCheckpoint verifies checkpoint/key encoding and supplies populated words.
//
// Version:
//   - 2026-09-11: Added.
func (r *windowReader) DynamicValuesByKeysAtCheckpoint(ctx context.Context, parent sui.Address, cp sui.CheckpointSequenceNumber, keys []sui.DynamicFieldKey) ([]json.RawMessage, error) {
	r.calls = append(r.calls, len(keys))
	if cp != 123 {
		return nil, fmt.Errorf("failed to read fixture: checkpoint=invalid")
	}
	if len(r.calls) == r.failAt {
		return nil, r.failure
	}
	values := make([]json.RawMessage, len(keys))
	for i, key := range keys {
		if key.Type != "0x1::i32::I32" || len(key.BCS) != 4 {
			return nil, fmt.Errorf("failed to read fixture: key=invalid")
		}
		index := int32(binary.LittleEndian.Uint32(key.BCS))
		if parent[31] == 1 {
			bits := new(big.Int)
			if index == 0 {
				for b := 0; b < 70; b++ {
					bits.SetBit(bits, b, 1)
				}
			}
			values[i] = json.RawMessage(fmt.Sprintf("%q", bits.String()))
		} else {
			values[i] = json.RawMessage(`{"liquidity_net":{"bits":"340282366920938463463374607431768211455"}}`)
		}
	}
	if r.cancel != nil && len(r.calls) == 2 {
		r.cancel()
	}
	return values, nil
}
func unsigned(raw json.RawMessage) (*big.Int, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, fmt.Errorf("failed to parse fixture: number=invalid")
	}
	return n, nil
}

// TestReadBatchesCompleteWindow verifies every tick, signed net decoding and proactive movement detection.
//
// Version:
//   - 2026-09-11: Added.
func TestReadBatchesCompleteWindow(t *testing.T) {
	reader := &windowReader{}
	var bitmap, ticks sui.Address
	bitmap[31] = 1
	ticks[31] = 2
	words, nets, err := Read(context.Background(), reader, 123, bitmap, ticks, "0x1::i32::I32", 0, 1, unsigned)
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 3 || len(nets) != 70 || fmt.Sprint(reader.calls) != "[3 32 32 6]" {
		t.Fatalf("words=%d ticks=%d batches=%v", len(words), len(nets), reader.calls)
	}
	for i := int32(0); i < 70; i++ {
		if nets[i].Int64() != -1 {
			t.Fatal("incorrect signed tick", i)
		}
	}
	if Missing(200, 1, words, nets) {
		t.Fatal("populated window reported missing")
	}
	if !Missing(256, 1, words, nets) {
		t.Fatal("moving center did not request surrounding word")
	}
	delete(nets, 69)
	if !Missing(0, 1, words, nets) {
		t.Fatal("unvisited tick detail was not checked")
	}
	if Word(-1, 10) != -1 || Word(-2560, 10) != -1 || Word(-2561, 10) != -2 {
		t.Fatal("negative floor division")
	}
}

// TestReadRejectsPartialCapture verifies cancellation and later-batch failure discard every staged result.
//
// Version:
//   - 2026-09-11: Added.
func TestReadRejectsPartialCapture(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sentinel := errors.New("unavailable")
			reader := &windowReader{failAt: 3, failure: sentinel}
			if cancelled {
				reader.cancel = cancel
				sentinel = context.Canceled
			}
			var bitmap, ticks sui.Address
			bitmap[31] = 1
			ticks[31] = 2
			words, nets, err := Read(ctx, reader, 123, bitmap, ticks, "0x1::i32::I32", 0, 1, unsigned)
			if !errors.Is(err, sentinel) || words != nil || nets != nil {
				t.Fatalf("partial capture published: %v", err)
			}
			if cancelled && len(reader.calls) != 2 {
				t.Fatal("issued requests after cancellation")
			}
		})
	}
}
