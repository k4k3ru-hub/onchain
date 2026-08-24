package coin

import (
	"context"
	"fmt"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type TransactionSubscription interface {
	Recv(context.Context) (*onchainSui.TransactionNotification, error)
	Close()
}

type SubscribeTransactionsFunc func(context.Context, onchainSui.Address) (TransactionSubscription, error)

type TransferSubscriptionFilter struct {
	Address  onchainSui.Address
	CoinType string
}

type TransferSubscriber interface {
	SubscribeTransfers(context.Context, TransferSubscriptionFilter) (*TransferSubscription, error)
}

type StandardTransferSubscriber struct {
	client    *Client
	subscribe SubscribeTransactionsFunc
}

type standardGRPCClient interface {
	SubscribeTransactions(context.Context, onchainSui.Address) (*onchainSui.TransactionSubscription, error)
}

type TransferSubscription struct {
	client       *Client
	transactions TransactionSubscription
	filter       TransferSubscriptionFilter
	pending      []Transfer
}

// NewStandardTransferSubscriber creates a standard Sui gRPC transfer subscriber.
func NewStandardTransferSubscriber(client *Client, subscribe SubscribeTransactionsFunc) (*StandardTransferSubscriber, error) {
	if client == nil {
		return nil, fmt.Errorf("failed to create sui coin transfer subscriber: client=null")
	}
	if subscribe == nil {
		return nil, fmt.Errorf("failed to create sui coin transfer subscriber: subscribe_transactions=null")
	}
	return &StandardTransferSubscriber{client: client, subscribe: subscribe}, nil
}

// NewGRPCTransferSubscriber creates a transfer subscriber backed by the standard Sui gRPC client.
func NewGRPCTransferSubscriber(client *Client, grpcClient standardGRPCClient) (*StandardTransferSubscriber, error) {
	if grpcClient == nil {
		return nil, fmt.Errorf("failed to create sui grpc transfer subscriber: grpc_client=null")
	}
	subscriber, err := NewStandardTransferSubscriber(client, func(ctx context.Context, address onchainSui.Address) (TransactionSubscription, error) {
		return grpcClient.SubscribeTransactions(ctx, address)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create sui grpc transfer subscriber: %w", err)
	}
	return subscriber, nil
}

// SubscribeTransfers subscribes to transfers affecting an address.
func (s *StandardTransferSubscriber) SubscribeTransfers(ctx context.Context, filter TransferSubscriptionFilter) (*TransferSubscription, error) {
	if s == nil || s.client == nil || s.subscribe == nil {
		return nil, fmt.Errorf("failed to subscribe sui coin transfers: subscriber=null")
	}
	if filter.Address.IsZero() {
		return nil, fmt.Errorf("failed to subscribe sui coin transfers: address=empty")
	}
	if filter.CoinType != "" {
		normalized, err := onchainSui.NormalizeMoveType(filter.CoinType)
		if err != nil || !s.client.hasCoinType(normalized) {
			return nil, fmt.Errorf("failed to subscribe sui coin transfers: coin_type=not_configured")
		}
		filter.CoinType = normalized
	}
	if ctx == nil {
		ctx = context.Background()
	}
	transactions, err := s.subscribe(ctx, filter.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe sui coin transfers: %w", err)
	}
	return &TransferSubscription{client: s.client, transactions: transactions, filter: filter}, nil
}

// Recv waits for the next matching Sui coin transfer.
func (s *TransferSubscription) Recv(ctx context.Context) (*Transfer, error) {
	if s == nil || s.client == nil || s.transactions == nil {
		return nil, fmt.Errorf("failed to receive sui coin transfer: subscription=null")
	}
	for {
		if len(s.pending) > 0 {
			transfer := s.pending[0]
			s.pending = s.pending[1:]
			return &transfer, nil
		}
		notification, err := s.transactions.Recv(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to receive sui coin transfer: %w", err)
		}
		if notification == nil || notification.Effects == nil || !notification.Effects.Successful {
			continue
		}
		for _, transfer := range s.client.deriveTransfers(notification.Effects, s.filter.CoinType) {
			if transfer.From == s.filter.Address || transfer.To == s.filter.Address {
				s.pending = append(s.pending, transfer)
			}
		}
	}
}

// Close closes the underlying Sui transaction subscription.
func (s *TransferSubscription) Close() {
	if s != nil && s.transactions != nil {
		s.transactions.Close()
	}
}
