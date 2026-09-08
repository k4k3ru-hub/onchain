package sui

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
)

type DynamicValuePage struct {
	Values      []json.RawMessage
	HasNextPage bool
	NextCursor  string
}

// DynamicValuesAtCheckpoint reads a page of Move dynamic values at a fixed checkpoint.
// Only MoveValue fields are accepted; dynamic object fields are not substituted.
//
// Version:
//   - 2026-09-08: Added.
func (c *RPCClient) DynamicValuesAtCheckpoint(ctx context.Context, parent Address, checkpoint CheckpointSequenceNumber, first int, after string) (DynamicValuePage, error) {
	if c == nil || c.caller == nil {
		return DynamicValuePage{}, fmt.Errorf("failed to get sui dynamic values: rpc_client=null")
	}
	if parent.IsZero() || first < 1 || first > 100 || len(after) > 4096 {
		return DynamicValuePage{}, fmt.Errorf("failed to get sui dynamic values: parameters=invalid")
	}
	if err := checkpoint.Validate(); err != nil {
		return DynamicValuePage{}, fmt.Errorf("failed to get sui dynamic values: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cursor := ""
	if after != "" {
		cursor = fmt.Sprintf(", after: %q", after)
	}
	var result struct {
		Address *struct {
			Fields *struct {
				Nodes []struct {
					Value *struct {
						JSON json.RawMessage `json:"json"`
					} `json:"value"`
				} `json:"nodes"`
				Info struct {
					Next   bool   `json:"hasNextPage"`
					Cursor string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"dynamicFields"`
		} `json:"address"`
	}
	query := fmt.Sprintf(`query { address(address: %q, atCheckpoint: %d) { dynamicFields(first: %d%s) { nodes { value { ... on MoveValue { json } } } pageInfo { hasNextPage endCursor } } } }`, parent.String(), checkpoint.Uint64(), first, cursor)
	if err := c.caller.query(ctx, query, &result); err != nil {
		return DynamicValuePage{}, fmt.Errorf("failed to get sui dynamic values: %w", err)
	}
	if result.Address == nil || result.Address.Fields == nil {
		return DynamicValuePage{}, fmt.Errorf("failed to get sui dynamic values: fields=null")
	}
	f := result.Address.Fields
	p := DynamicValuePage{HasNextPage: f.Info.Next, NextCursor: f.Info.Cursor}
	if p.HasNextPage && (p.NextCursor == "" || p.NextCursor == after || len(f.Nodes) == 0) {
		return DynamicValuePage{}, fmt.Errorf("failed to get sui dynamic values: pagination=invalid")
	}
	for _, n := range f.Nodes {
		if n.Value == nil || len(n.Value.JSON) == 0 || string(n.Value.JSON) == "null" {
			return DynamicValuePage{}, fmt.Errorf("failed to get sui dynamic values: value=null")
		}
		p.Values = append(p.Values, append(json.RawMessage(nil), n.Value.JSON...))
	}
	return p, nil
}

// DynamicUint64ValuesAtCheckpoint reads Move values keyed by u64 in caller order.
// Missing fields remain nil so consumers can detect deleted entries.
//
// Version:
//   - 2026-09-08: Added.
func (c *RPCClient) DynamicUint64ValuesAtCheckpoint(ctx context.Context, parent Address, checkpoint CheckpointSequenceNumber, keys []uint64) ([]json.RawMessage, error) {
	if c == nil || c.caller == nil || parent.IsZero() || len(keys) == 0 || len(keys) > 50 {
		return nil, fmt.Errorf("failed to get sui keyed dynamic values: parameters=invalid")
	}
	if err := checkpoint.Validate(); err != nil {
		return nil, fmt.Errorf("failed to get sui keyed dynamic values: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	typed := make([]DynamicFieldKey, len(keys))
	for i, k := range keys {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], k)
		typed[i] = DynamicFieldKey{Type: "u64", BCS: append([]byte(nil), b[:]...)}
	}
	return c.DynamicValuesByKeysAtCheckpoint(ctx, parent, checkpoint, typed)
}

// DynamicFieldKey identifies a Move dynamic field using its owning type and BCS bytes.
type DynamicFieldKey struct {
	Type string
	BCS  []byte
}

// DynamicValuesByKeysAtCheckpoint reads typed Move dynamic values in caller order.
// Missing fields remain nil. All keys are resolved at the same checkpoint.
//
// Version:
//   - 2026-09-08: Added.
func (c *RPCClient) DynamicValuesByKeysAtCheckpoint(ctx context.Context, parent Address, checkpoint CheckpointSequenceNumber, keys []DynamicFieldKey) ([]json.RawMessage, error) {
	if c == nil || c.caller == nil || parent.IsZero() || len(keys) == 0 || len(keys) > 50 {
		return nil, fmt.Errorf("failed to get sui keyed dynamic values: parameters=invalid")
	}
	if err := checkpoint.Validate(); err != nil {
		return nil, fmt.Errorf("failed to get sui keyed dynamic values: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		if strings.TrimSpace(k.Type) == "" || len(k.Type) > 4096 || len(k.BCS) > 4096 {
			return nil, fmt.Errorf("failed to get sui keyed dynamic values: key=invalid")
		}
		parts[i] = fmt.Sprintf(`{type: %q, bcs: %q}`, k.Type, base64.StdEncoding.EncodeToString(k.BCS))
	}
	query := fmt.Sprintf(`query { address(address: %q, atCheckpoint: %d) { multiGetDynamicFields(keys: [%s]) { value { ... on MoveValue { json } } } } }`, parent.String(), checkpoint.Uint64(), strings.Join(parts, ","))
	var r struct {
		Address *struct {
			Fields []*struct {
				Value *struct {
					JSON json.RawMessage `json:"json"`
				} `json:"value"`
			} `json:"multiGetDynamicFields"`
		} `json:"address"`
	}
	if err := c.caller.query(ctx, query, &r); err != nil {
		return nil, fmt.Errorf("failed to get sui keyed dynamic values: %w", err)
	}
	if r.Address == nil || len(r.Address.Fields) != len(keys) {
		return nil, fmt.Errorf("failed to get sui keyed dynamic values: fields=invalid")
	}
	out := make([]json.RawMessage, len(keys))
	for i, f := range r.Address.Fields {
		if f == nil {
			continue
		}
		if f.Value == nil || len(f.Value.JSON) == 0 || string(f.Value.JSON) == "null" {
			return nil, fmt.Errorf("failed to get sui keyed dynamic values: value=null")
		}
		out[i] = append(json.RawMessage(nil), f.Value.JSON...)
	}
	return out, nil
}
