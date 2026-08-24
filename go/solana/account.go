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

type accountProvider interface {
	getAccount(context.Context, Address, Commitment) (*Account, error)
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
