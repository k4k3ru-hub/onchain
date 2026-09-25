package sui

import (
	"context"
	"fmt"
	"strconv"
)

type Epoch struct {
	ID                uint64
	ReferenceGasPrice uint64
}

// CurrentEpoch reads the current epoch and its reference gas price together.
//
// Version:
//   - 2026-09-24: Added.
func (c *RPCClient) CurrentEpoch(ctx context.Context) (Epoch, error) {
	if c == nil || c.caller == nil {
		return Epoch{}, fmt.Errorf("failed to get sui epoch: provider=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var response struct {
		Epoch *struct {
			ID    *uint64 `json:"epochId"`
			Price string  `json:"referenceGasPrice"`
		} `json:"epoch"`
	}
	if err := c.caller.query(ctx, "query { epoch { epochId referenceGasPrice } }", &response); err != nil {
		return Epoch{}, fmt.Errorf("failed to get sui epoch: %w", err)
	}
	if response.Epoch == nil || response.Epoch.ID == nil {
		return Epoch{}, fmt.Errorf("failed to get sui epoch: epoch=null")
	}
	price, err := strconv.ParseUint(response.Epoch.Price, 10, 64)
	if err != nil || price == 0 {
		return Epoch{}, fmt.Errorf("failed to get sui epoch: reference_gas_price=invalid")
	}
	return Epoch{ID: *response.Epoch.ID, ReferenceGasPrice: price}, nil
}
