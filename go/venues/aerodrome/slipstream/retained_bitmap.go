package slipstream

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
)

func (c *StateCache) readBitmapWindow(ctx context.Context, s *poolSnapshot, lower, upper int32, budget *int) error {
	count := int(upper - lower + 1)
	values := make([][]byte, count)
	if batch, ok := c.rpc.(batchStateReader); ok {
		if *budget < count {
			return fmt.Errorf("failed to capture retained bitmap: %w", errStateReadBudget)
		}
		*budget -= count // Budget logical reads, not HTTP round trips.
		calls := make([][]byte, count)
		for i := range calls {
			word := big.NewInt(int64(lower) + int64(i))
			if word.Sign() < 0 {
				word.Add(word, power2(256))
			}
			calls[i] = append(append([]byte(nil), crypto.Keccak256([]byte("tickBitmap(int16)"))[:4]...), word.FillBytes(make([]byte, 32))...)
		}
		results, failures, err := batch.ReadContracts(ctx, c.pool, calls, s.header.Number)
		if err != nil {
			return fmt.Errorf("failed to capture retained bitmap batch: %w", err)
		}
		if len(results) != count || len(failures) != count {
			return fmt.Errorf("failed to capture retained bitmap batch: results=invalid")
		}
		for i, failure := range failures {
			if failure != nil {
				return fmt.Errorf("failed to capture retained bitmap: %w: word=%d", failure, int64(lower)+int64(i))
			}
		}
		values = results
	} else {
		for i := range values {
			word := int64(lower) + int64(i)
			value, err := c.read(ctx, s, "tickBitmap(int16)", big.NewInt(word), budget)
			if err != nil {
				return fmt.Errorf("failed to capture retained bitmap: %w: word=%d", err, word)
			}
			values[i] = value
		}
	}
	for i, value := range values {
		if len(value) != 32 {
			return fmt.Errorf("failed to capture retained bitmap: result=invalid word=%d", int64(lower)+int64(i))
		}
	}
	for i, value := range values {
		s.words[lower+int32(i)] = new(big.Int).SetBytes(value)
	}
	return nil
}
