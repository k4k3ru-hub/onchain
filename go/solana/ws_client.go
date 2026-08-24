package solana

import (
	"context"
	"fmt"
	"strings"
	"sync"

	solanaSDK "github.com/gagliardetto/solana-go"
	solanaRPC "github.com/gagliardetto/solana-go/rpc"
	solanaWS "github.com/gagliardetto/solana-go/rpc/ws"
)

type WSConfig struct {
	URL        string
	Commitment Commitment
}
type WSClient struct {
	provider  logSubscriptionProvider
	closer    interface{ Close() }
	closeOnce sync.Once
}
type logSubscriptionProvider interface {
	subscribeLogs(Address, Commitment) (logReceiver, error)
}
type logReceiver interface {
	Recv(context.Context) (*Log, error)
	Unsubscribe()
}

type sdkWSAdapter struct{ client *solanaWS.Client }
type sdkLogReceiver struct{ subscription *solanaWS.LogSubscription }

// NewWSClient creates a Solana WebSocket RPC client.
func NewWSClient(ctx context.Context, config WSConfig) (*WSClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(config.URL) == "" {
		return nil, fmt.Errorf("failed to create solana websocket client: ws_url=empty")
	}
	if err := config.Commitment.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create solana websocket client: %w", err)
	}
	client, err := solanaWS.Connect(ctx, strings.TrimSpace(config.URL))
	if err != nil {
		return nil, fmt.Errorf("failed to create solana websocket client: %w", err)
	}
	adapter := &sdkWSAdapter{client: client}
	return &WSClient{provider: adapter, closer: client}, nil
}

// Close closes the Solana WebSocket client.
func (c *WSClient) Close() {
	if c != nil {
		c.closeOnce.Do(func() {
			if c.closer != nil {
				c.closer.Close()
			}
		})
	}
}

// SubscribeLogs subscribes to logs for transactions mentioning an address.
func (c *WSClient) SubscribeLogs(address Address) (*LogSubscription, error) {
	if c == nil || c.provider == nil {
		return nil, fmt.Errorf("failed to subscribe solana logs: provider=null")
	}
	if address.IsZero() {
		return nil, fmt.Errorf("failed to subscribe solana logs: address=empty")
	}
	receiver, err := c.provider.subscribeLogs(address, CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe solana logs: %w", err)
	}
	return &LogSubscription{receiver: receiver}, nil
}

type LogSubscription struct {
	receiver  logReceiver
	closeOnce sync.Once
}

// Recv waits for the next Solana log notification.
func (s *LogSubscription) Recv(ctx context.Context) (*Log, error) {
	if s == nil || s.receiver == nil {
		return nil, fmt.Errorf("failed to receive solana log: subscription=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	log, err := s.receiver.Recv(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to receive solana log: %w", err)
	}
	return log, nil
}

// Close unsubscribes the Solana log subscription.
func (s *LogSubscription) Close() {
	if s != nil {
		s.closeOnce.Do(func() {
			if s.receiver != nil {
				s.receiver.Unsubscribe()
			}
		})
	}
}

func (a *sdkWSAdapter) subscribeLogs(address Address, commitment Commitment) (logReceiver, error) {
	var publicKey solanaSDK.PublicKey
	copy(publicKey[:], address[:])
	subscription, err := a.client.LogsSubscribeMentions(publicKey, solanaRPC.CommitmentType(commitment))
	if err != nil {
		return nil, err
	}
	return &sdkLogReceiver{subscription: subscription}, nil
}

func (r *sdkLogReceiver) Recv(ctx context.Context) (*Log, error) {
	value, err := r.subscription.Recv(ctx)
	if err != nil {
		return nil, err
	}
	var signature Signature
	copy(signature[:], value.Value.Signature[:])
	return &Log{Signature: signature, Slot: Slot(value.Context.Slot), Messages: append([]string(nil), value.Value.Logs...), Failed: value.Value.Err != nil}, nil
}
func (r *sdkLogReceiver) Unsubscribe() { r.subscription.Unsubscribe() }
