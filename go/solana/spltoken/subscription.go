package spltoken

import (
	"context"
	"fmt"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type LogSubscription interface {
	Recv(context.Context) (*onchainSolana.Log, error)
	Close()
}

type SubscribeLogsFunc func(onchainSolana.Address) (LogSubscription, error)

type TransferSubscriber interface {
	SubscribeTransfers(context.Context, TransferFilter) (*TransferSubscription, error)
}

type StandardTransferSubscriber struct {
	source    transactionSource
	subscribe SubscribeLogsFunc
}

type TransferSubscription struct {
	source  transactionSource
	logs    LogSubscription
	filter  TransferFilter
	pending []TransferEvent
}

// NewStandardTransferSubscriber creates a standard RPC/WebSocket transfer subscriber.
func NewStandardTransferSubscriber(source transactionSource, subscribe SubscribeLogsFunc) (*StandardTransferSubscriber, error) {
	if source == nil {
		return nil, fmt.Errorf("failed to create standard transfer subscriber: transaction_source=null")
	}
	if subscribe == nil {
		return nil, fmt.Errorf("failed to create standard transfer subscriber: subscribe_logs=null")
	}
	return &StandardTransferSubscriber{source: source, subscribe: subscribe}, nil
}

// SubscribeTransfers subscribes to SPL Token transfers mentioning the filtered address.
func (s *StandardTransferSubscriber) SubscribeTransfers(_ context.Context, filter TransferFilter) (*TransferSubscription, error) {
	if s == nil || s.source == nil || s.subscribe == nil {
		return nil, fmt.Errorf("failed to subscribe spl token transfers: subscriber=null")
	}
	if filter.Address.IsZero() {
		return nil, fmt.Errorf("failed to subscribe spl token transfers: address=empty")
	}
	logs, err := s.subscribe(filter.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe spl token transfers: %w", err)
	}
	return &TransferSubscription{source: s.source, logs: logs, filter: filter}, nil
}

// Recv waits for the next matching SPL Token transfer.
func (s *TransferSubscription) Recv(ctx context.Context) (*TransferEvent, error) {
	if s == nil || s.logs == nil || s.source == nil {
		return nil, fmt.Errorf("failed to receive spl token transfer: subscription=null")
	}
	for {
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
			return &event, nil
		}
		log, err := s.logs.Recv(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to receive spl token transfer: %w", err)
		}
		if log == nil || log.Failed {
			continue
		}
		transaction, err := s.source.Transaction(ctx, log.Signature)
		if err != nil {
			return nil, fmt.Errorf("failed to receive spl token transfer: %w", err)
		}
		for _, event := range deriveTransfers(transaction, s.filter.Mint) {
			if event.From == s.filter.Address || event.To == s.filter.Address {
				s.pending = append(s.pending, event)
			}
		}
	}
}

// Close closes the underlying log subscription.
func (s *TransferSubscription) Close() {
	if s != nil && s.logs != nil {
		s.logs.Close()
	}
}
