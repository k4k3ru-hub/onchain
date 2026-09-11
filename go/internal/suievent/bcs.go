// Package suievent decodes fixed-layout Move events without network access.
package suievent

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
)

type Field struct {
	Name string
	Type string
}

// JSON decodes a complete fixed-layout BCS event for the existing JSON validators.
//
// Version:
//   - 2026-09-11: Added.
func JSON(data []byte, fields []Field) (json.RawMessage, error) {
	values := make(map[string]any, len(fields))
	offset := 0
	for _, field := range fields {
		size := 0
		switch field.Type {
		case "bool":
			size = 1
		case "u32":
			size = 4
		case "u64":
			size = 8
		case "u128":
			size = 16
		case "address":
			size = 32
		default:
			return nil, fmt.Errorf("failed to decode sui event bcs: field_type=invalid")
		}
		if len(data)-offset < size {
			return nil, fmt.Errorf("failed to decode sui event bcs: data=too_short field=%q", field.Name)
		}
		value := data[offset : offset+size]
		offset += size
		switch field.Type {
		case "bool":
			if value[0] > 1 {
				return nil, fmt.Errorf("failed to decode sui event bcs: boolean=invalid field=%q", field.Name)
			}
			values[field.Name] = value[0] == 1
		case "address":
			values[field.Name] = "0x" + hex.EncodeToString(value)
		default:
			var reversed [16]byte
			for i := range value {
				reversed[size-1-i] = value[i]
			}
			values[field.Name] = new(big.Int).SetBytes(reversed[:size]).String()
		}
	}
	if offset != len(data) {
		return nil, fmt.Errorf("failed to decode sui event bcs: data=too_long expected_length=%d actual_length=%d", offset, len(data))
	}
	result, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to encode sui event json: %w", err)
	}
	return result, nil
}
