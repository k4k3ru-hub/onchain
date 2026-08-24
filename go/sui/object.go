package sui

import (
	"context"
	"encoding/json"
	"fmt"
)

type Object struct {
	Address Address
	Version uint64
	Digest  ObjectDigest
	BCS     string
	Move    *MoveObject
	Package bool
}

type MoveObject struct {
	Type string
	JSON json.RawMessage
}

// Object returns the latest version of a Sui object.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - address: object address.
//
// Returns:
//   - SDK-owned object.
//   - Retrieval or validation error.
//
// Version:
//   - 2026-08-23: Added.
func (c *RPCClient) Object(ctx context.Context, address Address) (*Object, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to get sui object: rpc_client=null")
	}
	if c.caller == nil {
		return nil, fmt.Errorf("failed to get sui object: object_provider=null")
	}
	if address.IsZero() {
		return nil, fmt.Errorf("failed to get sui object: address=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result struct {
		Object *struct {
			Address   string `json:"address"`
			Version   uint64 `json:"version"`
			Digest    string `json:"digest"`
			ObjectBCS string `json:"objectBcs"`
			Move      *struct {
				Contents *struct {
					Type *struct {
						Representation string `json:"repr"`
					} `json:"type"`
					JSON json.RawMessage `json:"json"`
				} `json:"contents"`
			} `json:"asMoveObject"`
			Package *struct {
				Address string `json:"address"`
			} `json:"asMovePackage"`
		} `json:"object"`
	}
	query := fmt.Sprintf(`query { object(address: %q) { address version digest objectBcs asMoveObject { contents { type { repr } json } } asMovePackage { address } } }`, address.String())
	if err := c.caller.query(ctx, query, &result); err != nil {
		return nil, fmt.Errorf("failed to get sui object: %w", err)
	}
	if result.Object == nil {
		return nil, fmt.Errorf("failed to get sui object: object=null")
	}
	returnedAddress, err := ParseAddress(result.Object.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get sui object: %w", err)
	}
	if returnedAddress != address {
		return nil, fmt.Errorf("failed to get sui object: address=mismatch")
	}
	digest, err := ParseObjectDigest(result.Object.Digest)
	if err != nil {
		return nil, fmt.Errorf("failed to get sui object: %w", err)
	}
	object := &Object{Address: returnedAddress, Version: result.Object.Version, Digest: digest, BCS: result.Object.ObjectBCS, Package: result.Object.Package != nil}
	if result.Object.Move != nil {
		if result.Object.Move.Contents == nil || result.Object.Move.Contents.Type == nil {
			return nil, fmt.Errorf("failed to get sui object: move_contents=invalid")
		}
		object.Move = &MoveObject{Type: result.Object.Move.Contents.Type.Representation, JSON: append(json.RawMessage(nil), result.Object.Move.Contents.JSON...)}
	}
	if object.Move == nil && !object.Package {
		return nil, fmt.Errorf("failed to get sui object: object_kind=invalid")
	}
	return object, nil
}
