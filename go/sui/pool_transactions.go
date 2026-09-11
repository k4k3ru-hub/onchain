package sui

import (
	"context"
	"errors"
	"fmt"
	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/proto"
)

// TransactionReceiver supplies transaction notifications and releases its resources.
type TransactionReceiver interface {
	Recv() (*TransactionNotification, error)
	Close()
}

// NewTransactionSubscription wraps an injected notification receiver.
//
// Version:
//   - 2026-09-11: Added for local pool-state fanout.
func NewTransactionSubscription(receiver TransactionReceiver) (*TransactionSubscription, error) {
	if receiver == nil {
		return nil, fmt.Errorf("failed to create sui transaction subscription: receiver=null")
	}
	return &TransactionSubscription{receiver: receiver}, nil
}

// SubscribePoolTransactions includes live events and changed objects in one pool stream.
// Event and object decoding failures are independent fields on the notification.
//
// Version:
//   - 2026-09-11: Added.
func (c *GRPCClient) SubscribePoolTransactions(ctx context.Context, object Address) (*TransactionSubscription, error) {
	if c == nil || ctx == nil || object.IsZero() {
		return nil, fmt.Errorf("failed to subscribe sui pool transactions: parameters=invalid")
	}
	provider, ok := c.transactionProvider.(interface {
		subscribePoolTransactions(context.Context, Address, bool) (liveTransactionReceiver, error)
	})
	if !ok {
		return nil, fmt.Errorf("failed to subscribe sui pool transactions: provider=unsupported")
	}
	receiver, err := provider.subscribePoolTransactions(ctx, object, true)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe sui pool transactions: %w", err)
	}
	return NewTransactionSubscription(receiver)
}

func decodeTransactionEvents(tx *rpcv2.ExecutedTransaction) ([]LiveEvent, error) {
	var events []LiveEvent
	var failures []error
	for index, value := range tx.GetEvents().GetEvents() {
		if value == nil {
			failures = append(failures, fmt.Errorf("failed to decode sui transaction event: event=null event_index=%d", index))
			continue
		}
		item := proto.Clone(value).(*rpcv2.Event)
		item.Checkpoint = tx.Checkpoint
		item.TransactionDigest = tx.Digest
		item.TransactionIndex = tx.TransactionIndex
		position := uint32(index)
		item.EventIndex = &position
		parsed, err := decodeGRPCEvent(item)
		if err != nil {
			failures = append(failures, fmt.Errorf("failed to decode sui transaction event: %w: event_index=%d", err, index))
			continue
		}
		events = append(events, *parsed)
	}
	return events, errors.Join(failures...)
}
