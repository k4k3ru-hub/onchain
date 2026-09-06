package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
)

// PriceLevel is the exact [price, size] tuple used by Arcus order books.
type PriceLevel [2]string

// UnmarshalJSON rejects truncated or oversized tuples instead of losing book data.
//
// Version:
//   - 2026-09-06: Added.
func (p *PriceLevel) UnmarshalJSON(data []byte) error {
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("failed to decode price level: %w", safety.Redact(err))
	}
	if len(values) != 2 || values[0] == "" || values[1] == "" {
		return fmt.Errorf("failed to decode price level: tuple=invalid")
	}
	*p = PriceLevel{values[0], values[1]}
	return nil
}
