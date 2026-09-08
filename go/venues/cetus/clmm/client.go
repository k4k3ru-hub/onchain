package clmm

import (
	"context"
	"fmt"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type objectProvider interface {
	Object(context.Context, onchainSui.Address) (*onchainSui.Object, error)
}

type eventProvider interface {
	Events(context.Context, onchainSui.EventQuery) (onchainSui.EventPage, error)
}

type eventSubscriber interface {
	SubscribeEvents(context.Context, onchainSui.LiveEventFilter) (*onchainSui.EventSubscription, error)
}

type Client struct {
	deployment Deployment
	objects    objectProvider
	events     eventProvider
	subscriber eventSubscriber
	quoter     *Quoter
}

type SwapQuery struct {
	Pool             *onchainSui.Address
	AfterCheckpoint  *onchainSui.CheckpointSequenceNumber
	AtCheckpoint     *onchainSui.CheckpointSequenceNumber
	BeforeCheckpoint *onchainSui.CheckpointSequenceNumber
	First            int
	After            string
}

type SwapPage struct {
	Swaps       []Swap
	HasNextPage bool
	NextCursor  string
}

type SwapNotification struct {
	Swap       *Swap
	Checkpoint *onchainSui.CheckpointSequenceNumber
}

type SwapSubscription struct {
	subscription *onchainSui.EventSubscription
	pool         *onchainSui.Address
}

// NewClient composes a Cetus CLMM client from Sui RPC and gRPC clients.
//
// Parameters:
//   - deployment: Cetus deployment.
//   - rpcClient: Sui GraphQL RPC client used for objects and historical events.
//   - grpcClient: Sui gRPC client used for simulation and live events.
//
// Returns:
//   - Composed Cetus client.
//   - Construction error.
//
// Version:
//   - 2026-08-30: Added.
func NewClient(deployment Deployment, rpcClient *onchainSui.RPCClient, grpcClient *onchainSui.GRPCClient) (*Client, error) {
	if rpcClient == nil {
		return nil, fmt.Errorf("failed to create cetus clmm client: rpc_client=null")
	}
	if grpcClient == nil {
		return nil, fmt.Errorf("failed to create cetus clmm client: grpc_client=null")
	}
	return composeClient(deployment, rpcClient, rpcClient, grpcClient, grpcClient)
}

func composeClient(deployment Deployment, objects objectProvider, events eventProvider, subscriber eventSubscriber, simulator Simulator) (*Client, error) {
	if err := deployment.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create cetus clmm client: %w", err)
	}
	if objects == nil {
		return nil, fmt.Errorf("failed to create cetus clmm client: object_provider=null")
	}
	if events == nil {
		return nil, fmt.Errorf("failed to create cetus clmm client: event_provider=null")
	}
	if subscriber == nil {
		return nil, fmt.Errorf("failed to create cetus clmm client: event_subscriber=null")
	}
	quoter, err := NewQuoter(deployment, simulator)
	if err != nil {
		return nil, fmt.Errorf("failed to create cetus clmm client: %w", err)
	}
	return &Client{deployment: deployment, objects: objects, events: events, subscriber: subscriber, quoter: quoter}, nil
}

// Pool gets and parses the latest Cetus CLMM pool state.
//
// Parameters:
//   - ctx: Request context.
//   - address: Pool object address.
//
// Returns:
//   - Parsed pool state.
//   - Retrieval or parse error.
//
// Version:
//   - 2026-08-30: Added.
func (c *Client) Pool(ctx context.Context, address onchainSui.Address) (Pool, error) {
	if c == nil || c.objects == nil {
		return Pool{}, fmt.Errorf("failed to get cetus clmm pool: client=null")
	}
	object, err := c.objects.Object(ctx, address)
	if err != nil {
		return Pool{}, fmt.Errorf("failed to get cetus clmm pool: %w", err)
	}
	pool, err := ParsePool(object)
	if err != nil {
		return Pool{}, fmt.Errorf("failed to get cetus clmm pool: %w", err)
	}
	if pool == nil {
		return Pool{}, fmt.Errorf("failed to get cetus clmm pool: pool=null")
	}
	return *pool, nil
}

// QuoteExactInput simulates one exact-input Cetus CLMM swap.
//
// Parameters:
//   - ctx: Request context.
//   - params: Quote parameters.
//
// Returns:
//   - Quote result.
//   - Quote or simulation error.
//
// Version:
//   - 2026-08-30: Added.
func (c *Client) QuoteExactInput(ctx context.Context, params QuoteExactInputParams) (QuoteResult, error) {
	if c == nil || c.quoter == nil {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm exact input: client=null")
	}
	return c.quoter.QuoteExactInput(ctx, params)
}

// QuoteExactOutput simulates one exact-output Cetus CLMM swap.
//
// Parameters:
//   - ctx: Request context.
//   - params: Quote parameters.
//
// Returns:
//   - Quote result.
//   - Quote or simulation error.
//
// Version:
//   - 2026-08-31: Added.
func (c *Client) QuoteExactOutput(ctx context.Context, params QuoteExactOutputParams) (QuoteResult, error) {
	if c == nil || c.quoter == nil {
		return QuoteResult{}, fmt.Errorf("failed to quote cetus clmm exact output: client=null")
	}
	return c.quoter.QuoteExactOutput(ctx, params)
}

// Swaps gets a page of historical Cetus CLMM swaps.
//
// Parameters:
//   - ctx: Request context.
//   - query: Pool, checkpoint, and pagination filters.
//
// Returns:
//   - Parsed swap page.
//   - Retrieval or parse error.
//
// Version:
//   - 2026-08-30: Added.
func (c *Client) Swaps(ctx context.Context, query SwapQuery) (SwapPage, error) {
	if c == nil || c.events == nil {
		return SwapPage{}, fmt.Errorf("failed to get cetus clmm swaps: client=null")
	}
	if query.Pool != nil && query.Pool.IsZero() {
		return SwapPage{}, fmt.Errorf("failed to get cetus clmm swaps: pool=empty")
	}
	page, err := c.events.Events(ctx, onchainSui.EventQuery{
		Filter: onchainSui.EventFilter{
			Type:             c.swapEventType(),
			AfterCheckpoint:  query.AfterCheckpoint,
			AtCheckpoint:     query.AtCheckpoint,
			BeforeCheckpoint: query.BeforeCheckpoint,
		},
		First: query.First,
		After: query.After,
	})
	if err != nil {
		return SwapPage{}, fmt.Errorf("failed to get cetus clmm swaps: %w", err)
	}
	swaps := make([]Swap, 0, len(page.Events))
	for _, event := range page.Events {
		swap, err := ParseSwapEvent(event)
		if err != nil {
			return SwapPage{}, fmt.Errorf("failed to get cetus clmm swaps: %w", err)
		}
		if query.Pool == nil || swap.Pool == *query.Pool {
			swaps = append(swaps, swap)
		}
	}
	return SwapPage{Swaps: swaps, HasNextPage: page.HasNextPage, NextCursor: page.NextCursor}, nil
}

// SubscribeSwaps subscribes to live Cetus CLMM swap events.
//
// Parameters:
//   - ctx: Subscription context.
//   - pool: Optional pool filter; nil subscribes to all Cetus pools.
//
// Returns:
//   - Swap subscription.
//   - Subscription error.
//
// Version:
//   - 2026-08-30: Added.
func (c *Client) SubscribeSwaps(ctx context.Context, pool *onchainSui.Address) (*SwapSubscription, error) {
	if c == nil || c.subscriber == nil {
		return nil, fmt.Errorf("failed to subscribe cetus clmm swaps: client=null")
	}
	if pool != nil && pool.IsZero() {
		return nil, fmt.Errorf("failed to subscribe cetus clmm swaps: pool=empty")
	}
	subscription, err := c.subscriber.SubscribeEvents(ctx, onchainSui.LiveEventFilter{Type: c.swapEventType()})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe cetus clmm swaps: %w", err)
	}
	return &SwapSubscription{subscription: subscription, pool: pool}, nil
}

// Recv waits for the next matching swap or progress notification.
//
// Parameters:
//   - ctx: Receive context.
//
// Returns:
//   - Swap or checkpoint progress notification.
//   - Receive or parse error.
//
// Version:
//   - 2026-08-30: Added.
func (s *SwapSubscription) Recv(ctx context.Context) (*SwapNotification, error) {
	if s == nil || s.subscription == nil {
		return nil, fmt.Errorf("failed to receive cetus clmm swap: subscription=null")
	}
	for {
		notification, err := s.subscription.Recv(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to receive cetus clmm swap: %w", err)
		}
		if notification.Event == nil {
			return &SwapNotification{Checkpoint: notification.Watermark.Checkpoint}, nil
		}
		swap, err := ParseLiveSwapEvent(*notification.Event)
		if err != nil {
			return nil, fmt.Errorf("failed to receive cetus clmm swap: %w", err)
		}
		if s.pool == nil || swap.Pool == *s.pool {
			return &SwapNotification{Swap: &swap, Checkpoint: notification.Watermark.Checkpoint}, nil
		}
	}
}

// Close closes the underlying Sui event subscription.
//
// Version:
//   - 2026-08-30: Added.
func (s *SwapSubscription) Close() {
	if s != nil && s.subscription != nil {
		s.subscription.Close()
	}
}

func (c *Client) swapEventType() string {
	return c.deployment.Package.String() + "::" + c.deployment.PoolModule + "::SwapEvent"
}
