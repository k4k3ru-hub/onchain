package slipstream

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

const retainedTickBatchSize = 32

// retainedTickProgress belongs to one private, block-pinned capture. Only fully
// validated batches advance next; no partial data is published to quote readers.
type retainedTickProgress struct {
	staged  *poolSnapshot
	next    int
	started time.Time
}

// readWindowTicks fills every initialized tick in the bounded bitmap window.
// The separate budget is bounded by three words (768 ticks); it does not grant
// reference quotes permission to scan an unbounded number of additional words.
func (c *StateCache) readWindowTicks(ctx context.Context, s *poolSnapshot, lower, upper int32) error {
	return c.resumeWindowTicks(ctx, s, lower, upper, &retainedTickProgress{})
}

func (c *StateCache) resumeWindowTicks(ctx context.Context, s *poolSnapshot, lower, upper int32, progress *retainedTickProgress) (err error) {
	if lower > upper || upper-lower > 2*retainedWordRadius || s.spacing <= 0 {
		return fmt.Errorf("failed to capture retained ticks: window=invalid")
	}
	var ticks []int32
	for word := lower; word <= upper; word++ {
		bitmap := s.words[word]
		if bitmap == nil {
			return fmt.Errorf("failed to capture retained ticks: bitmap=null word=%d", word)
		}
		for bit := 0; bit < 256; bit++ {
			if bitmap.Bit(bit) == 0 {
				continue
			}
			tick := (int64(word)*256 + int64(bit)) * int64(s.spacing)
			if tick < -887272 || tick > 887272 {
				return fmt.Errorf("failed to capture retained ticks: tick=out_of_range word=%d", word)
			}
			ticks = append(ticks, int32(tick))
		}
	}
	if progress.staged == nil {
		progress.staged = &poolSnapshot{ticks: make(map[int32]*big.Int), gross: make(map[int32]*big.Int)}
		if progress.started.IsZero() {
			progress.started = time.Now()
		}
	}
	batchCount := (len(ticks) + retainedTickBatchSize - 1) / retainedTickBatchSize
	batchIndex, batchSize := 0, 0
	batchStarted := time.Now()
	defer func() {
		if err != nil {
			err = fmt.Errorf("failed to capture retained ticks: %w: pool_id=%q block_number=%d tick_count=%d completed_tick_count=%d batch_index=%d batch_count=%d batch_size=%d elapsed_seconds=%.3f batch_elapsed_seconds=%.3f", err, c.pool.Hex(), s.header.Number, len(ticks), progress.next, batchIndex, batchCount, batchSize, time.Since(progress.started).Seconds(), time.Since(batchStarted).Seconds())
		}
	}()
	budget := len(ticks)
	for start := progress.next; start < len(ticks); start += retainedTickBatchSize {
		batchIndex, batchSize = start/retainedTickBatchSize+1, min(retainedTickBatchSize, len(ticks)-start)
		batchStarted = time.Now()
		staged := &poolSnapshot{ticks: make(map[int32]*big.Int), gross: make(map[int32]*big.Int)}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("failed to capture retained ticks: %w", err)
		}
		chunk := ticks[start:min(start+retainedTickBatchSize, len(ticks))]
		if batch, ok := c.rpc.(batchStateReader); ok {
			calls := make([][]byte, len(chunk))
			for i, tick := range chunk {
				arg := big.NewInt(int64(tick))
				if arg.Sign() < 0 {
					arg.Add(arg, power2(256))
				}
				calls[i] = append(append([]byte(nil), crypto.Keccak256([]byte("ticks(int24)"))[:4]...), arg.FillBytes(make([]byte, 32))...)
			}
			values, failures, err := batch.ReadContracts(ctx, c.pool, calls, s.header.Number)
			if err != nil {
				return fmt.Errorf("failed to capture retained tick batch: %w", err)
			}
			if len(values) != len(chunk) || len(failures) != len(chunk) {
				return fmt.Errorf("failed to capture retained tick batch: results=invalid")
			}
			for i, tick := range chunk {
				if failures[i] != nil {
					return fmt.Errorf("failed to capture retained tick: %w: tick=%d", failures[i], tick)
				}
				if _, err := storeTickDetails(staged, tick, values[i]); err != nil {
					return fmt.Errorf("failed to capture retained tick: %w: tick=%d", err, tick)
				}
			}
		} else {
			for _, tick := range chunk {
				data, err := c.read(ctx, s, "ticks(int24)", big.NewInt(int64(tick)), &budget)
				if err != nil {
					return fmt.Errorf("failed to capture retained tick: %w: tick=%d", err, tick)
				}
				if _, err := storeTickDetails(staged, tick, data); err != nil {
					return err
				}
			}
		}
		for tick, net := range staged.ticks {
			progress.staged.ticks[tick], progress.staged.gross[tick] = net, staged.gross[tick]
		}
		progress.next = start + len(chunk)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("failed to capture retained ticks: %w", err)
	}
	if s.ticks == nil {
		s.ticks = make(map[int32]*big.Int)
	}
	if s.gross == nil {
		s.gross = make(map[int32]*big.Int)
	}
	for tick, net := range progress.staged.ticks {
		s.ticks[tick], s.gross[tick] = net, progress.staged.gross[tick]
	}
	return nil
}
