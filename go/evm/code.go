package evm

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

type codeReader interface {
	CodeAtHash(context.Context, common.Address, common.Hash) ([]byte, error)
}

// CodeAtHash reads contract runtime code at a required block hash without retries.
// Callers own chain identity, canonical block verification and acquisition budgets.
//
// Version:
//   - 2026-09-22: Added.
func (c *HTTPClient) CodeAtHash(ctx context.Context, address common.Address, blockHash common.Hash) ([]byte, error) {
	if c == nil || c.codeReader == nil {
		return nil, fmt.Errorf("failed to read evm code: code_reader=null")
	}
	if address == (common.Address{}) || blockHash == (common.Hash{}) {
		return nil, fmt.Errorf("failed to read evm code: address_or_block_hash=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	code, err := c.codeReader.CodeAtHash(ctx, address, blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to read evm code: %w", err)
	}
	return append([]byte(nil), code...), nil
}
