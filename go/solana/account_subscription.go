package solana

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/gagliardetto/solana-go"
	rpc "github.com/gagliardetto/solana-go/rpc"
	ws "github.com/gagliardetto/solana-go/rpc/ws"
)

type accountChangeProvider interface {
	subscribeAccountChanges(Address, Commitment) (accountChangeReceiver, error)
}
type accountChangeReceiver interface {
	Recv(context.Context) (Slot, error)
	Unsubscribe()
}

// AccountChangeSubscription reports slots whose account state must be refreshed.
type AccountChangeSubscription struct {
	receiver accountChangeReceiver
	once     sync.Once
}

// SubscribeAccountChanges subscribes to account changes at the configured commitment.
//
// Version:
//   - 2026-09-07: Added.
func (c *WSClient) SubscribeAccountChanges(address Address) (*AccountChangeSubscription, error) {
	if c == nil || c.accounts == nil {
		return nil, fmt.Errorf("failed to subscribe solana account changes: provider=null")
	}
	if address.IsZero() {
		return nil, fmt.Errorf("failed to subscribe solana account changes: address=empty")
	}
	r, err := c.accounts.subscribeAccountChanges(address, c.commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe solana account changes: %w", err)
	}
	if r == nil {
		return nil, fmt.Errorf("failed to subscribe solana account changes: subscription=null")
	}
	return &AccountChangeSubscription{receiver: r}, nil
}

// Recv waits for an account change and returns its context slot.
//
// Version:
//   - 2026-09-07: Added.
func (s *AccountChangeSubscription) Recv(ctx context.Context) (Slot, error) {
	if s == nil || s.receiver == nil {
		return 0, fmt.Errorf("failed to receive solana account change: subscription=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	slot, err := s.receiver.Recv(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to receive solana account change: %w", err)
	}
	if err := slot.Validate(); err != nil {
		return 0, fmt.Errorf("failed to receive solana account change: %w", err)
	}
	return slot, nil
}

// Close releases the account subscription once.
//
// Version:
//   - 2026-09-07: Added.
func (s *AccountChangeSubscription) Close() {
	if s != nil {
		s.once.Do(func() {
			if s.receiver != nil {
				s.receiver.Unsubscribe()
			}
		})
	}
}

type sdkAccountChanges struct{ subscription *ws.AccountSubscription }

func (a *sdkWSAdapter) subscribeAccountChanges(address Address, commitment Commitment) (accountChangeReceiver, error) {
	sub, err := a.client.AccountSubscribe(sdk.PublicKey(address), rpc.CommitmentType(commitment))
	if err != nil {
		return nil, err
	}
	return &sdkAccountChanges{sub}, nil
}

// Recv returns the notification slot.
//
// Version:
//   - 2026-09-07: Added.
func (s *sdkAccountChanges) Recv(ctx context.Context) (Slot, error) {
	v, err := s.subscription.Recv(ctx)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, fmt.Errorf("failed to receive account notification: notification=null")
	}
	return Slot(v.Context.Slot), nil
}

// Unsubscribe releases the transport subscription.
//
// Version:
//   - 2026-09-07: Added.
func (s *sdkAccountChanges) Unsubscribe() { s.subscription.Unsubscribe() }
