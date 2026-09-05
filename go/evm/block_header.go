// block_header.go
package evm

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type blockHeaderByHasher interface {
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
}

type blockHeaderByNumberer interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

type BlockHeader struct {
	Number        uint64
	Hash          common.Hash
	ParentHash    common.Hash
	Timestamp     uint64
	BaseFeePerGas *big.Int
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
	result, err := convertBlockHeader("failed to get evm header by hash", header)
	if err != nil {
		return BlockHeader{}, err
	}
	if result.Hash != hash {
		return BlockHeader{}, fmt.Errorf("failed to get evm header by hash: block_hash=mismatch")
	}
	return result, nil
}

// HeaderByNumber gets an EVM block header by its required block number.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - blockNumber: block number, including zero for the genesis block.
//
// Returns:
//   - SDK-owned EVM block header.
//   - Header retrieval or validation error.
//
// Version:
//   - 2026-08-29: Added.
func (c *HTTPClient) HeaderByNumber(ctx context.Context, blockNumber uint64) (BlockHeader, error) {
	const operation = "failed to get evm header by number"
	if c == nil {
		return BlockHeader{}, fmt.Errorf("%s: evm_http_client=null", operation)
	}
	if c.blockHeaderByNumberer == nil {
		return BlockHeader{}, fmt.Errorf("%s: http_eth_client=null", operation)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	header, err := c.blockHeaderByNumberer.HeaderByNumber(ctx, new(big.Int).SetUint64(blockNumber))
	if err != nil {
		return BlockHeader{}, fmt.Errorf("%s: %w", operation, err)
	}
	result, err := convertBlockHeader(operation, header)
	if err != nil {
		return BlockHeader{}, err
	}
	if result.Number != blockNumber {
		return BlockHeader{}, fmt.Errorf("%s: block_number=mismatch", operation)
	}
	return result, nil
}

// LatestHeader gets the latest EVM block header.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//
// Returns:
//   - SDK-owned latest EVM block header.
//   - Header retrieval or validation error.
//
// Version:
//   - 2026-08-29: Added.
func (c *HTTPClient) LatestHeader(ctx context.Context) (BlockHeader, error) {
	const operation = "failed to get latest evm header"
	if c == nil {
		return BlockHeader{}, fmt.Errorf("%s: evm_http_client=null", operation)
	}
	if c.blockHeaderByNumberer == nil {
		return BlockHeader{}, fmt.Errorf("%s: http_eth_client=null", operation)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	header, err := c.blockHeaderByNumberer.HeaderByNumber(ctx, nil)
	if err != nil {
		return BlockHeader{}, fmt.Errorf("%s: %w", operation, err)
	}
	return convertBlockHeader(operation, header)
}

func convertBlockHeader(operation string, header *types.Header) (BlockHeader, error) {
	if header == nil {
		return BlockHeader{}, fmt.Errorf("%s: block_header=null", operation)
	}
	if header.Number == nil {
		return BlockHeader{}, fmt.Errorf("%s: block_number=null", operation)
	}
	if !header.Number.IsUint64() {
		return BlockHeader{}, fmt.Errorf("%s: block_number=out_of_range", operation)
	}

	result := BlockHeader{
		Number:     header.Number.Uint64(),
		Hash:       header.Hash(),
		ParentHash: header.ParentHash,
		Timestamp:  header.Time,
	}
	if header.BaseFee != nil {
		result.BaseFeePerGas = new(big.Int).Set(header.BaseFee)
	}
	return result, nil
}
