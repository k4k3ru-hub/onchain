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

type AddressSignature struct {
	Signature Signature
	Slot      Slot
	Failed    bool
}

type SignaturesForAddressQuery struct {
	Address Address
	Limit   int
	Before  *Signature
	Until   *Signature
}
type addressSignaturesProvider interface {
	getAddressSignatures(context.Context, SignaturesForAddressQuery, Commitment) ([]AddressSignature, error)
}

// SignaturesForAddress returns recent transaction signatures mentioning an address.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - address: account address mentioned by returned transactions.
//   - limit: maximum result count from 1 through 1000.
//
// Returns:
//   - Transaction signatures ordered newest to oldest.
//   - Request error.
//
// Version:
//   - 2026-09-01: Delegated to the pagination-aware query API.
func (c *RPCClient) SignaturesForAddress(ctx context.Context, address Address, limit int) ([]Signature, error) {
	values, err := c.SignaturesForAddressPage(ctx, SignaturesForAddressQuery{Address: address, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("failed to get solana signatures for address: %w", err)
	}
	result := make([]Signature, len(values))
	for i := range values {
		result[i] = values[i].Signature
	}
	return result, nil
}

// SignaturesForAddressPage returns one pagination-aware page of transactions mentioning an address.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - query: address, page size, and optional exclusive before and until cursors.
//
// Returns:
//   - Transaction metadata ordered newest to oldest.
//   - Request error.
//
// Version:
//   - 2026-09-01: Added.
func (c *RPCClient) SignaturesForAddressPage(ctx context.Context, query SignaturesForAddressQuery) ([]AddressSignature, error) {
	if c == nil || c.addressSignaturesProvider == nil {
		return nil, fmt.Errorf("failed to get solana signatures for address page: provider=null")
	}
	if query.Address.IsZero() {
		return nil, fmt.Errorf("failed to get solana signatures for address page: address=empty")
	}
	if query.Limit < 1 || query.Limit > 1000 {
		return nil, fmt.Errorf("failed to get solana signatures for address page: limit=out_of_range min_value=1 max_value=1000")
	}
	if query.Before != nil && query.Before.IsZero() {
		return nil, fmt.Errorf("failed to get solana signatures for address page: before=invalid")
	}
	if query.Until != nil && query.Until.IsZero() {
		return nil, fmt.Errorf("failed to get solana signatures for address page: until=invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	values, err := c.addressSignaturesProvider.getAddressSignatures(ctx, query, c.config.Commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to get solana signatures for address page: %w", err)
	}
	return values, nil
}
