package evm

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

type headSubscriber interface {
	SubscribeNewHead(context.Context, chan<- *types.Header) (ethereum.Subscription, error)
}

type HeaderSubscription struct {
	ctx     context.Context
	cancel  context.CancelFunc
	headers <-chan *types.Header
	sub     ethereum.Subscription
	once    sync.Once
}

// SubscribeHeaders subscribes to block headers over the existing WebSocket connection.
// The caller must close the subscription. Notifications do not prove that all
// logs for a block have arrived; consumers must match logs by block hash.
//
// Version:
//   - 2026-09-09: Added.
func (c *WSClient) SubscribeHeaders(ctx context.Context) (*HeaderSubscription, error) {
	const operation = "failed to subscribe evm headers"
	if c == nil || c.headSubscriber == nil {
		return nil, fmt.Errorf("%s: header_subscriber=null", operation)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	headers := make(chan *types.Header, 128)
	sub, err := c.headSubscriber.SubscribeNewHead(ctx, headers)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	if sub == nil {
		cancel()
		return nil, fmt.Errorf("%s: subscription=null", operation)
	}
	return &HeaderSubscription{ctx: ctx, cancel: cancel, headers: headers, sub: sub}, nil
}

// Recv receives an SDK-owned header without issuing HTTP requests.
// Receive errors require the caller to close and recreate the subscription.
//
// Version:
//   - 2026-09-09: Added.
func (s *HeaderSubscription) Recv(ctx context.Context) (BlockHeader, error) {
	const operation = "failed to receive evm header"
	if s == nil || s.sub == nil {
		return BlockHeader{}, fmt.Errorf("%s: subscription=null", operation)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Reject already-closed subscriptions even if notifications remain buffered.
	if err := s.ctx.Err(); err != nil {
		return BlockHeader{}, fmt.Errorf("%s: %w", operation, err)
	}
	// Transport failure invalidates queued notifications. Do not drain an old
	// connection after its loss has already been reported.
	select {
	case err, ok := <-s.sub.Err():
		s.Close()
		if !ok || err == nil {
			return BlockHeader{}, fmt.Errorf("%s: subscription closed", operation)
		}
		return BlockHeader{}, fmt.Errorf("%s: %w", operation, err)
	default:
	}
	select {
	case <-ctx.Done():
		return BlockHeader{}, fmt.Errorf("%s: %w", operation, ctx.Err())
	case <-s.ctx.Done():
		return BlockHeader{}, fmt.Errorf("%s: %w", operation, s.ctx.Err())
	case err, ok := <-s.sub.Err():
		s.Close()
		if !ok || err == nil {
			return BlockHeader{}, fmt.Errorf("%s: subscription closed", operation)
		}
		return BlockHeader{}, fmt.Errorf("%s: %w", operation, err)
	case header, ok := <-s.headers:
		if !ok {
			s.Close()
			return BlockHeader{}, fmt.Errorf("%s: header channel closed", operation)
		}
		result, err := convertBlockHeader(operation, header)
		if err != nil {
			s.Close()
		}
		return result, err
	}
}

// Close releases the header subscription and interrupts pending receives.
//
// Version:
//   - 2026-09-09: Added.
func (s *HeaderSubscription) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.sub != nil {
			s.sub.Unsubscribe()
		}
	})
}
