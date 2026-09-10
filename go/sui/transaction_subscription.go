package sui

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type TransactionNotification struct {
	Effects       *TransactionEffects
	Watermark     EventWatermark
	ObjectChanges []ObjectChange
}

type TransactionSubscription struct {
	receiver  liveTransactionReceiver
	closeOnce sync.Once
}

type grpcTransactionReceiver struct {
	includeObjects bool
	stream         interface {
		Recv() (*rpcv2.SubscribeTransactionsResponse, error)
	}
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// SubscribeTransactions subscribes to transactions affecting an address.
//
// Version:
//   - 2026-09-10: Retain optional checkpoint-local transaction position.
//   - 2026-08-23: Added.
func (c *GRPCClient) SubscribeTransactions(ctx context.Context, address Address) (*TransactionSubscription, error) {
	if c == nil || c.transactionProvider == nil {
		return nil, fmt.Errorf("failed to subscribe sui transactions: transaction_provider=null")
	}
	if address.IsZero() {
		return nil, fmt.Errorf("failed to subscribe sui transactions: address=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	receiver, err := c.transactionProvider.subscribeTransactions(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe sui transactions: %w", err)
	}
	return &TransactionSubscription{receiver: receiver}, nil
}

// Recv waits for the next transaction or progress-only notification.
func (s *TransactionSubscription) Recv(ctx context.Context) (*TransactionNotification, error) {
	if s == nil || s.receiver == nil {
		return nil, fmt.Errorf("failed to receive sui transaction: subscription=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	type result struct {
		notification *TransactionNotification
		err          error
	}
	resultChannel := make(chan result, 1)
	go func() {
		notification, err := s.receiver.Recv()
		resultChannel <- result{notification: notification, err: err}
	}()
	select {
	case <-ctx.Done():
		s.Close()
		return nil, fmt.Errorf("failed to receive sui transaction: %w", ctx.Err())
	case received := <-resultChannel:
		if received.err != nil {
			return nil, fmt.Errorf("failed to receive sui transaction: %w", received.err)
		}
		if received.notification == nil {
			return nil, fmt.Errorf("failed to receive sui transaction: notification=null")
		}
		return received.notification, nil
	}
}

// Close closes the Sui transaction subscription.
func (s *TransactionSubscription) Close() {
	if s != nil {
		s.closeOnce.Do(func() {
			if s.receiver != nil {
				s.receiver.Close()
			}
		})
	}
}

func (a *grpcAdapter) subscribeTransactions(ctx context.Context, address Address) (liveTransactionReceiver, error) {
	streamContext, cancel := context.WithCancel(ctx)
	addressValue := address.String()
	filter := &rpcv2.TransactionFilter{Terms: []*rpcv2.TransactionTerm{{Literals: []*rpcv2.TransactionLiteral{{
		Predicate: &rpcv2.TransactionLiteral_AffectedAddress{AffectedAddress: &rpcv2.AffectedAddressFilter{Address: &addressValue}},
	}}}}}
	request := &rpcv2.SubscribeTransactionsRequest{
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"digest", "effects.status", "checkpoint", "transaction_index", "timestamp", "balance_changes"}},
		Filter:   filter,
	}
	stream, err := a.client.SubscribeTransactions(streamContext, request)
	if err != nil {
		cancel()
		return nil, err
	}
	return &grpcTransactionReceiver{stream: stream, cancel: cancel}, nil
}

func (a *grpcAdapter) subscribeObjectTransactions(ctx context.Context, address Address) (liveTransactionReceiver, error) {
	streamContext, cancel := context.WithCancel(ctx)
	addressValue := address.String()
	filter := &rpcv2.TransactionFilter{Terms: []*rpcv2.TransactionTerm{{Literals: []*rpcv2.TransactionLiteral{{
		Predicate: &rpcv2.TransactionLiteral_AffectedObject{AffectedObject: &rpcv2.AffectedObjectFilter{ObjectId: &addressValue}},
	}}}}}
	request := &rpcv2.SubscribeTransactionsRequest{
		ReadMask: &fieldmaskpb.FieldMask{Paths: []string{"digest", "effects.status", "checkpoint", "transaction_index", "timestamp"}},
		Filter:   filter,
	}
	stream, err := a.client.SubscribeTransactions(streamContext, request)
	if err != nil {
		cancel()
		return nil, err
	}
	return &grpcTransactionReceiver{stream: stream, cancel: cancel}, nil
}

func (r *grpcTransactionReceiver) Recv() (*TransactionNotification, error) {
	response, err := r.stream.Recv()
	if err != nil {
		return nil, err
	}
	if response == nil || response.Watermark == nil || len(response.Watermark.Cursor) == 0 {
		return nil, fmt.Errorf("failed to decode sui grpc transaction: watermark=invalid")
	}
	notification := &TransactionNotification{Watermark: EventWatermark{Cursor: append([]byte(nil), response.Watermark.Cursor...)}}
	if response.Watermark.Checkpoint != nil {
		checkpoint := CheckpointSequenceNumber(*response.Watermark.Checkpoint)
		notification.Watermark.Checkpoint = &checkpoint
	}
	if response.Transaction != nil {
		effects, err := decodeGRPCTransaction(response.Transaction)
		if err != nil {
			return nil, err
		}
		notification.Effects = effects
		if r.includeObjects {
			changes, err := decodeObjectChanges(response.Transaction)
			if err != nil {
				return nil, err
			}
			notification.ObjectChanges = changes
		}
	}
	return notification, nil
}

func (r *grpcTransactionReceiver) Close() {
	if r != nil {
		r.closeOnce.Do(func() {
			if r.cancel != nil {
				r.cancel()
			}
		})
	}
}

func decodeGRPCTransaction(value *rpcv2.ExecutedTransaction) (*TransactionEffects, error) {
	if value.Digest == nil || value.Checkpoint == nil || value.Effects == nil || value.Effects.Status == nil {
		return nil, fmt.Errorf("failed to decode sui grpc transaction: transaction_fields=null")
	}
	digest, err := ParseTransactionDigest(value.GetDigest())
	if err != nil {
		return nil, fmt.Errorf("failed to decode sui grpc transaction: %w", err)
	}
	checkpoint := CheckpointSequenceNumber(value.GetCheckpoint())
	effects := &TransactionEffects{Digest: digest, Checkpoint: &checkpoint, Successful: value.Effects.Status.GetSuccess()}
	if value.TransactionIndex != nil {
		index := *value.TransactionIndex
		effects.TransactionIndex = &index
	}
	if value.Timestamp != nil {
		timestamp := value.Timestamp.AsTime()
		effects.Timestamp = &timestamp
	}
	if executionError := value.Effects.Status.GetError(); executionError != nil {
		message := executionError.GetKind().String()
		effects.Error = &message
	}
	for _, change := range value.BalanceChanges {
		if change == nil || change.Address == nil || change.CoinType == nil || change.Amount == nil || strings.TrimSpace(change.GetCoinType()) == "" {
			return nil, fmt.Errorf("failed to decode sui grpc transaction: balance_change=invalid")
		}
		address, err := ParseAddress(change.GetAddress())
		if err != nil {
			return nil, fmt.Errorf("failed to decode sui grpc transaction: %w", err)
		}
		amount, ok := new(big.Int).SetString(change.GetAmount(), 10)
		if !ok || amount.Sign() == 0 {
			return nil, fmt.Errorf("failed to decode sui grpc transaction: balance_change=invalid")
		}
		effects.BalanceChanges = append(effects.BalanceChanges, BalanceChange{Address: address, CoinType: change.GetCoinType(), Amount: amount})
	}
	return effects, nil
}

// SubscribeObjectTransactions subscribes to transactions affecting a pool or configuration object.
// It includes liquidity and administrative changes, not only Swap events.
// Watermarks are progress hints; callers must verify state before extending quote validity.
//
// Version:
//   - 2026-09-10: Retain optional checkpoint-local transaction position.
//   - 2026-09-09: Added.
func (c *GRPCClient) SubscribeObjectTransactions(ctx context.Context, object Address) (*TransactionSubscription, error) {
	if c == nil || object.IsZero() {
		return nil, fmt.Errorf("failed to subscribe sui object changes: parameters=invalid")
	}
	provider, ok := c.transactionProvider.(interface {
		subscribeObjectTransactions(context.Context, Address) (liveTransactionReceiver, error)
	})
	if !ok {
		return nil, fmt.Errorf("failed to subscribe sui object changes: provider=unsupported")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	receiver, err := provider.subscribeObjectTransactions(ctx, object)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe sui object changes: %w", err)
	}
	if receiver == nil {
		return nil, fmt.Errorf("failed to subscribe sui object changes: receiver=null")
	}
	return &TransactionSubscription{receiver: receiver}, nil
}
