package spot

import "fmt"

// SwapFeePPM reads the current input fee from immutable retained state without quoting a quantity.
// The fee includes the protocol share and uses the requested token direction.
//
// Version:
//   - 2026-09-28: Added.
func (s *QuoteSnapshot) SwapFeePPM(inputIsToken0 bool) (uint32, error) {
	if s == nil || s.cache.retained == nil {
		return 0, fmt.Errorf("failed to read swap fee: snapshot=null")
	}
	fee := uint64(s.cache.retained.pool.FeeRate)
	if fee >= 1_000_000 {
		return 0, fmt.Errorf("failed to read swap fee: fee=out_of_range")
	}
	return uint32(fee), nil
}
