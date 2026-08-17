// block_header.go
package evm

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type blockHeaderByHasher interface {
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
}

type BlockHeader struct {
	Number     uint64
	Hash       common.Hash
	ParentHash common.Hash
	Timestamp  uint64
}

// HeaderByHash gets an EVM block header by hash using the HTTP RPC client.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - hash: block hash.
//
// Returns:
//   - SDK-owned EVM block header.
//   - Header retrieval or validation error.
//
// Version:
//   - 2026-08-17: Added.
func (c *HTTPClient) HeaderByHash(ctx context.Context, hash common.Hash) (BlockHeader, error) {
	if c == nil {
		return BlockHeader{}, fmt.Errorf("failed to get evm header by hash: evm_http_client=null")
	}
	if c.blockHeaderByHasher == nil {
		return BlockHeader{}, fmt.Errorf("failed to get evm header by hash: http_eth_client=null")
	}
	if hash == (common.Hash{}) {
		return BlockHeader{}, fmt.Errorf("failed to get evm header by hash: block_hash=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	header, err := c.blockHeaderByHasher.HeaderByHash(ctx, hash)
	if err != nil {
		return BlockHeader{}, fmt.Errorf("failed to get evm header by hash: %w", err)
	}
	if header == nil {
		return BlockHeader{}, fmt.Errorf("failed to get evm header by hash: block_header=null")
	}
	if header.Number == nil {
		return BlockHeader{}, fmt.Errorf("failed to get evm header by hash: block_number=null")
	}
	if !header.Number.IsUint64() {
		return BlockHeader{}, fmt.Errorf("failed to get evm header by hash: block_number=out_of_range")
	}

	headerHash := header.Hash()
	if headerHash != hash {
		return BlockHeader{}, fmt.Errorf("failed to get evm header by hash: block_hash=mismatch")
	}

	return BlockHeader{
		Number:     header.Number.Uint64(),
		Hash:       headerHash,
		ParentHash: header.ParentHash,
		Timestamp:  header.Time,
	}, nil
}
