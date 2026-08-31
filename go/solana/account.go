package solana

import (
	"context"
	"fmt"
)

type Account struct {
	Address    Address
	Owner      Address
	Lamports   uint64
	Executable bool
	Data       []byte
}

// AccountSnapshot contains accounts returned from one Solana RPC context.
type AccountSnapshot struct {
	Slot     Slot
	Accounts []*Account
}

type accountProvider interface {
	getAccount(context.Context, Address, Commitment) (*Account, error)
}

type accountSnapshotProvider interface {
	getAccountSnapshot(context.Context, []Address, Commitment) (*AccountSnapshot, error)
}

// Account returns Solana account information.
func (c *RPCClient) Account(ctx context.Context, address Address) (*Account, error) {
	if c == nil || c.accountProvider == nil {
		return nil, fmt.Errorf("failed to get solana account: account_provider=null")
	}
	if address.IsZero() {
		return nil, fmt.Errorf("failed to get solana account: address=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	account, err := c.accountProvider.getAccount(ctx, address, c.config.Commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to get solana account: %w", err)
	}
	if account == nil {
		return nil, fmt.Errorf("failed to get solana account: account=null")
	}
	account.Data = append([]byte(nil), account.Data...)
	return account, nil
}

// AccountSnapshot returns Solana accounts observed in one RPC context.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - addresses: account addresses to read in positional order.
//
// Returns:
//   - Account snapshot and its RPC context slot.
//   - Account read error.
//
// Version:
//   - 2026-08-31: Added.
func (c *RPCClient) AccountSnapshot(ctx context.Context, addresses []Address) (*AccountSnapshot, error) {
	if c == nil || c.accountSnapshotProvider == nil {
		return nil, fmt.Errorf("failed to get solana account snapshot: account_snapshot_provider=null")
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("failed to get solana account snapshot: addresses=empty")
	}
	for _, address := range addresses {
		if address.IsZero() {
			return nil, fmt.Errorf("failed to get solana account snapshot: address=empty")
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	snapshot, err := c.accountSnapshotProvider.getAccountSnapshot(ctx, append([]Address(nil), addresses...), c.config.Commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to get solana account snapshot: %w", err)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("failed to get solana account snapshot: snapshot=null")
	}
	if err := snapshot.Slot.Validate(); err != nil {
		return nil, fmt.Errorf("failed to get solana account snapshot: %w", err)
	}
	if len(snapshot.Accounts) != len(addresses) {
		return nil, fmt.Errorf("failed to get solana account snapshot: account_count=invalid actual_length=%d expected_length=%d", len(snapshot.Accounts), len(addresses))
	}
	for _, account := range snapshot.Accounts {
		if account != nil {
			account.Data = append([]byte(nil), account.Data...)
		}
	}
	snapshot.Accounts = append([]*Account(nil), snapshot.Accounts...)
	return snapshot, nil
}
