// Package websocket composes Arcus public subscriptions and a streaming transport.
package websocket

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket/protocol"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/websocket/subscriptions"
)

const RobinhoodMainnetURL = "wss://api.arcus.xyz/v1/ws"

const RobinhoodTestnetURL = "wss://api.testnet.arcus.xyz/v1/ws"

// Connection permits one reader, concurrent sends/pings, and concurrent Close.
// Cancellation of a read or write may close the connection.
type Connection interface {
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
	Ping(context.Context) error
	Close() error
}
type Dialer interface {
	Dial(context.Context, string) (Connection, error)
}
type ClientParams struct {
	EndpointURL     string
	Dialer          Dialer
	ConnectTimeout  time.Duration
	WriteTimeout    time.Duration
	PingInterval    time.Duration
	MaxMessageBytes int64
}
type Client struct {
	OrderBook        *subscriptions.Client
	OrderBookUpdates *subscriptions.Client
	BBO              *subscriptions.Client
	Trades           *subscriptions.Client
	Markets          *subscriptions.Client
	OraclePrices     *subscriptions.Client
	PredictedFunding *subscriptions.Client
	params           ClientParams
	mu               sync.Mutex
	session          *session
	readMu           sync.Mutex
}
type session struct {
	conn     Connection
	done     chan struct{}
	finished chan struct{}
	once     sync.Once
	mu       sync.Mutex
	cause    error
	closeErr error
}

// NewClient composes subscription clients without opening a network connection.
// Zero options select Robinhood mainnet, 10s connect, 5s writes, and 30s pings.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(p ClientParams) (*Client, error) {
	if p.EndpointURL == "" {
		p.EndpointURL = RobinhoodMainnetURL
	}
	u, err := safety.Endpoint(p.EndpointURL, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create arcus websocket client: %w", err)
	}
	if p.ConnectTimeout < 0 || p.WriteTimeout < 0 || p.PingInterval < 0 || p.PingInterval >= 2*time.Minute || p.MaxMessageBytes < 0 || p.MaxMessageBytes > 1<<30 {
		return nil, fmt.Errorf("failed to create arcus websocket client: limits=out_of_range")
	}
	if p.ConnectTimeout == 0 {
		p.ConnectTimeout = 10 * time.Second
	}
	if p.WriteTimeout == 0 {
		p.WriteTimeout = 5 * time.Second
	}
	if p.PingInterval == 0 {
		p.PingInterval = 30 * time.Second
	}
	if p.MaxMessageBytes == 0 {
		p.MaxMessageBytes = 8 << 20
	}
	p.EndpointURL = u.String()
	if p.Dialer == nil {
		p.Dialer = &defaultDialer{maxBytes: p.MaxMessageBytes, timeout: p.ConnectTimeout}
	}
	if safety.IsNil(p.Dialer) {
		return nil, fmt.Errorf("failed to create arcus websocket client: dialer=null")
	}
	c := &Client{params: p}
	for _, group := range []struct {
		channel string
		target  **subscriptions.Client
	}{{"l2Orderbook", &c.OrderBook}, {"l2OrderbookUpdates", &c.OrderBookUpdates}, {"bbo", &c.BBO}, {"trades", &c.Trades}, {"markets", &c.Markets}, {"oraclePrices", &c.OraclePrices}, {"predictedFunding", &c.PredictedFunding}} {
		client, err := subscriptions.NewClient(c, group.channel)
		if err != nil {
			return nil, fmt.Errorf("failed to create arcus websocket client: %w", err)
		}
		*group.target = client
	}
	return c, nil
}

// Connect opens a session and starts keepalive; subscriptions must be sent explicitly.
// The context bounds dialing only. Close owns the established session lifetime.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Connect(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("failed to connect arcus websocket: client=null")
	}
	if ctx == nil {
		return fmt.Errorf("failed to connect arcus websocket: context=null")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return fmt.Errorf("failed to connect arcus websocket: session=invalid")
	}
	ctx, cancel := context.WithTimeout(ctx, c.params.ConnectTimeout)
	defer cancel()
	conn, err := c.params.Dialer.Dial(ctx, c.params.EndpointURL)
	if err != nil {
		return fmt.Errorf("failed to connect arcus websocket: %w", safety.Redact(err))
	}
	if safety.IsNil(conn) {
		return fmt.Errorf("failed to connect arcus websocket: connection=null")
	}
	s := &session{conn: conn, done: make(chan struct{}), finished: make(chan struct{})}
	c.session = s
	go c.keepalive(s)
	return nil
}
func (c *Client) active() (*session, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to get websocket session: client=null")
	}
	c.mu.Lock()
	s := c.session
	c.mu.Unlock()
	if s == nil {
		return nil, fmt.Errorf("failed to get websocket session: session=null")
	}
	select {
	case <-s.done:
		if err := s.failure(); err != nil {
			return nil, fmt.Errorf("failed to get websocket session: %w", err)
		}
		return nil, fmt.Errorf("failed to get websocket session: session=invalid")
	default:
		return s, nil
	}
}
func (s *session) stop(cause error) {
	s.once.Do(func() {
		s.mu.Lock()
		s.cause = cause
		s.mu.Unlock()
		close(s.done)
		err := s.conn.Close()
		s.mu.Lock()
		if err != nil {
			s.closeErr = fmt.Errorf("failed to close websocket connection: %w", safety.Redact(err))
		}
		s.mu.Unlock()
	})
}
func (s *session) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return errors.Join(s.cause, s.closeErr)
}

func (s *session) fail(err error) error {
	s.stop(err)
	failure := s.failure()
	if errors.Is(failure, err) {
		return failure
	}
	// Concurrent failures must not hide the current operation's inspectable error.
	return errors.Join(err, failure)
}
func (c *Client) keepalive(s *session) {
	defer close(s.finished)
	ticker := time.NewTicker(c.params.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), c.params.WriteTimeout)
			err := s.conn.Ping(ctx)
			cancel()
			if err != nil {
				s.stop(fmt.Errorf("failed to ping arcus websocket: %w", safety.Redact(err)))
				return
			}
		}
	}
}

// Send sends a protocol request on the current session with a bounded write.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Send(ctx context.Context, data []byte) error {
	if ctx == nil {
		return fmt.Errorf("failed to send arcus request: context=null")
	}
	s, err := c.active()
	if err != nil {
		return fmt.Errorf("failed to send arcus request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.params.WriteTimeout)
	defer cancel()
	if err := s.conn.Write(ctx, data); err != nil {
		writeErr := fmt.Errorf("failed to write arcus request: %w", safety.Redact(err))
		return fmt.Errorf("failed to send arcus request: %w", s.fail(writeErr))
	}
	return nil
}

// Recv reads and decodes the next message, including subscription acknowledgements.
// No local book is reconstructed. Callers must reconcile per-market lastSequenceId values.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Recv(ctx context.Context) (*protocol.Message, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to receive arcus message: client=null")
	}
	if ctx == nil {
		return nil, fmt.Errorf("failed to receive arcus message: context=null")
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	s, err := c.active()
	if err != nil {
		return nil, fmt.Errorf("failed to receive arcus message: %w", err)
	}
	data, err := s.conn.Read(ctx)
	if err != nil {
		readErr := fmt.Errorf("failed to read arcus message: %w", safety.Redact(err))
		return nil, fmt.Errorf("failed to receive arcus message: %w", s.fail(readErr))
	}
	if int64(len(data)) > c.params.MaxMessageBytes {
		s.stop(fmt.Errorf("failed to read arcus message: body=too_long"))
		return nil, s.failure()
	}
	message, err := protocol.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to receive arcus message: %w", err)
	}
	return message, nil
}

// Close stops keepalive and closes the session; repeated calls are harmless.
// A new Connect requires explicit resubscription and a fresh order-book snapshot.
//
// Version:
//   - 2026-09-06: Added.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.session
	if s == nil {
		return nil
	}
	s.stop(nil)
	<-s.finished
	c.session = nil
	if err := s.failure(); err != nil {
		return fmt.Errorf("failed to close arcus websocket: %w", err)
	}
	return nil
}
