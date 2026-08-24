package solana

import (
	"context"
	"fmt"
)

type Log struct {
	Signature Signature
	Slot      Slot
	Messages  []string
	Failed    bool
}

type addressSignature struct {
	signature Signature
	slot      Slot
	failed    bool
}
type addressSignaturesProvider interface {
	getAddressSignatures(context.Context, Address, Commitment, int) ([]addressSignature, error)
}

// SignaturesForAddress returns recent transaction signatures mentioning an address.
func (c *RPCClient) SignaturesForAddress(ctx context.Context, address Address, limit int) ([]Signature, error) {
	if c == nil || c.addressSignaturesProvider == nil {
		return nil, fmt.Errorf("failed to get solana signatures for address: provider=null")
	}
	if address.IsZero() {
		return nil, fmt.Errorf("failed to get solana signatures for address: address=empty")
	}
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("failed to get solana signatures for address: limit=out_of_range min_value=1 max_value=1000")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	values, err := c.addressSignaturesProvider.getAddressSignatures(ctx, address, c.config.Commitment, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get solana signatures for address: %w", err)
	}
	result := make([]Signature, len(values))
	for i := range values {
		result[i] = values[i].signature
	}
	return result, nil
}
