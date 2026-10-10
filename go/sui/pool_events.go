package sui

import (
	"context"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// SubscribePoolEvents streams ordered BCS events and transaction times for a pool.
// It excludes Object state and rendered event JSON. The caller decodes event BCS
// using the emitting package's schema and filters events to its target pool.
// Progress-only notifications, context cancellation and Close are supported.
//
// Parameters:
//   - ctx: Subscription lifetime.
//   - object: Pool object affected by the transaction.
//
// Returns:
//   - Transaction metadata, progress and successful transaction events.
//   - Validation or transport error.
//
// Version:
//   - 2026-10-08: Added independently of Pool state subscriptions.
func (c *GRPCClient) SubscribePoolEvents(ctx context.Context, object Address) (*TransactionSubscription, error) {
	if c == nil || ctx == nil || object.IsZero() {
		return nil, fmt.Errorf("failed to subscribe sui pool events: parameters=invalid")
	}
	provider, ok := c.transactionProvider.(interface {
		subscribePoolEvents(context.Context, Address) (liveTransactionReceiver, error)
	})
	if !ok {
		return nil, fmt.Errorf("failed to subscribe sui pool events: provider=unsupported")
	}
	receiver, err := provider.subscribePoolEvents(ctx, object)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe sui pool events: %w", err)
	}
	subscription, err := NewTransactionSubscription(receiver)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe sui pool events: %w", err)
	}
	return subscription, nil
}

func (a *grpcAdapter) subscribePoolEvents(ctx context.Context, object Address) (liveTransactionReceiver, error) {
	ctx, cancel := context.WithCancel(ctx)
	address := object.String()
	request := &rpcv2.SubscribeTransactionsRequest{
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{
			"digest", "effects.status", "checkpoint", "transaction_index", "timestamp",
			"events.events.package_id", "events.events.module", "events.events.sender",
			"events.events.event_type", "events.events.contents",
		}},
		Filter: &rpcv2.TransactionFilter{Terms: []*rpcv2.TransactionTerm{{Literals: []*rpcv2.TransactionLiteral{{
			Predicate: &rpcv2.TransactionLiteral_AffectedObject{AffectedObject: &rpcv2.AffectedObjectFilter{ObjectId: &address}},
		}}}}},
	}
	stream, err := a.client.SubscribeTransactions(ctx, request)
	if err != nil {
		cancel()
		return nil, err
	}
	if stream == nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe sui pool events: stream=null")
	}
	return &grpcTransactionReceiver{stream: stream, cancel: cancel, includeEvents: true}, nil
}
