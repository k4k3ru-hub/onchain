package solana

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	sdkWS "github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/gorilla/websocket"
)

// streamWS owns a single reader and bounded subscription queues. Any received
// frame proves liveness; a quiet connection is checked with frequent pings.
type streamWS struct {
	conn       *websocket.Conn
	mu         sync.Mutex
	writer     sync.Mutex
	pending    map[uint32]*streamSub
	active     map[uint64]*streamSub
	next       uint32
	done       chan struct{}
	err        error
	once       sync.Once
	idle, ping time.Duration
}
type streamSub struct {
	client *streamWS
	ready  chan struct{}
	queue  chan json.RawMessage
	done   chan struct{}
	once   sync.Once
	id     uint64
	method string
	err    error
}

func dialStreamWS(ctx context.Context, url string, idle, ping time.Duration) (*streamWS, error) {
	d := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 10 * time.Second}
	conn, resp, err := d.DialContext(ctx, url, nil)
	if err != nil {
		if resp != nil && resp.Body != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to dial solana stream: %w", errors.Join(err, closeErr))
			}
		}
		return nil, fmt.Errorf("failed to dial solana stream: %w", err)
	}
	c := &streamWS{conn: conn, pending: make(map[uint32]*streamSub), active: make(map[uint64]*streamSub), done: make(chan struct{}), idle: idle, ping: ping}
	if err := conn.SetReadDeadline(time.Now().Add(idle)); err != nil {
		c.stop(err)
		return nil, err
	}
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(idle)) })
	go c.read()
	go c.heartbeat()
	return c, nil
}
func (c *streamWS) stop(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		close(c.done)
		c.mu.Unlock()
		if closeErr := c.conn.Close(); closeErr != nil {
			c.mu.Lock()
			if c.err == nil {
				c.err = closeErr
			}
			c.mu.Unlock()
		}
	})
}

// Close releases the connection and wakes every subscriber.
//
// Version:
//   - 2026-09-09: Added.
func (c *streamWS) Close()         { c.stop(fmt.Errorf("failed to receive solana stream: connection closed")) }
func (c *streamWS) failure() error { c.mu.Lock(); defer c.mu.Unlock(); return c.err }
func (c *streamWS) heartbeat() {
	ticker := time.NewTicker(c.ping)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				c.stop(fmt.Errorf("failed to ping solana stream: %w", err))
				return
			}
		}
	}
}
func (c *streamWS) write(v any) error {
	c.writer.Lock()
	defer c.writer.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return c.conn.WriteJSON(v)
}
func (c *streamWS) read() {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.stop(fmt.Errorf("failed to read solana stream: %w", err))
			return
		}
		if err := c.conn.SetReadDeadline(time.Now().Add(c.idle)); err != nil {
			c.stop(err)
			return
		}
		var msg struct {
			ID     *uint32         `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code int `json:"code"`
			} `json:"error"`
			Params *struct {
				Subscription uint64          `json:"subscription"`
				Result       json.RawMessage `json:"result"`
			} `json:"params"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			c.stop(fmt.Errorf("failed to decode solana stream: %w", err))
			return
		}
		c.mu.Lock()
		if msg.ID != nil {
			s := c.pending[*msg.ID]
			if s != nil {
				delete(c.pending, *msg.ID)
				if msg.Error != nil {
					s.err = fmt.Errorf("failed to subscribe solana stream: rpc_code=%d", msg.Error.Code)
				} else if err := json.Unmarshal(msg.Result, &s.id); err != nil {
					s.err = fmt.Errorf("failed to decode solana subscription: %w", err)
				} else if string(msg.Result) == "null" {
					s.err = fmt.Errorf("failed to decode solana subscription: subscription=null")
				} else if c.active[s.id] != nil {
					s.err = fmt.Errorf("failed to subscribe solana stream: duplicate subscription id")
					close(s.ready)
					c.mu.Unlock()
					c.stop(s.err)
					return
				} else {
					c.active[s.id] = s
				}
				close(s.ready)
			}
			c.mu.Unlock()
			continue
		}
		if msg.Params != nil {
			s := c.active[msg.Params.Subscription]
			if s != nil {
				select {
				case s.queue <- msg.Params.Result:
				default:
					c.mu.Unlock()
					c.stop(fmt.Errorf("failed to retain solana notification: queue=too_long"))
					return
				}
			}
		}
		c.mu.Unlock()
	}
}
func (c *streamWS) subscribe(method string, params any) (*streamSub, error) {
	s := &streamSub{client: c, ready: make(chan struct{}), queue: make(chan json.RawMessage, 256), done: make(chan struct{}), method: method}
	c.mu.Lock()
	select {
	case <-c.done:
		err := c.err
		c.mu.Unlock()
		return nil, err
	default:
	}
	if c.next >= 1<<31-1 {
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to subscribe solana stream: request_id=out_of_range")
	}
	c.next++
	id := c.next
	c.pending[id] = s
	c.mu.Unlock()
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method + "Subscribe", "params": params}); err != nil {
		c.stop(err)
		return nil, err
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-s.ready:
		if s.err != nil {
			return nil, s.err
		}
		return s, nil
	case <-c.done:
		return nil, c.failure()
	case <-timer.C:
		c.stop(fmt.Errorf("failed to subscribe solana stream: response timeout"))
		return nil, c.failure()
	}
}
func (s *streamSub) recv(ctx context.Context) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-s.client.done:
		return nil, s.client.failure()
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, fmt.Errorf("failed to receive solana stream: subscription closed")
	case <-s.client.done:
		return nil, s.client.failure()
	case v := <-s.queue:
		return v, nil
	}
}

// Unsubscribe removes the local consumer and releases its remote subscription.
//
// Version:
//   - 2026-09-09: Added.
func (s *streamSub) Unsubscribe() {
	s.once.Do(func() {
		c := s.client
		c.mu.Lock()
		delete(c.active, s.id)
		close(s.done)
		c.mu.Unlock()
		select {
		case <-c.done:
			return
		default:
		}
		if err := c.write(map[string]any{"jsonrpc": "2.0", "id": 0, "method": s.method + "Unsubscribe", "params": []any{s.id}}); err != nil {
			c.stop(err)
		}
	})
}

type streamLogs struct{ *streamSub }
type streamAccount struct {
	*streamSub
	address Address
}

func (c *streamWS) subscribeLogs(a Address, commitment Commitment) (logReceiver, error) {
	s, err := c.subscribe("logs", []any{map[string]any{"mentions": []string{a.String()}}, map[string]any{"commitment": string(commitment)}})
	if err != nil {
		return nil, err
	}
	return &streamLogs{s}, nil
}
func (c *streamWS) subscribeAccountChanges(a Address, commitment Commitment) (accountChangeReceiver, error) {
	s, err := c.subscribe("account", []any{a.String(), map[string]any{"commitment": string(commitment), "encoding": "base64"}})
	if err != nil {
		return nil, err
	}
	return &streamAccount{s, a}, nil
}

// Recv decodes one streamed log notification.
//
// Version:
//   - 2026-09-09: Added.
func (s *streamLogs) Recv(ctx context.Context) (*Log, error) {
	data, err := s.recv(ctx)
	if err != nil {
		return nil, err
	}
	var v sdkWS.LogResult
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("failed to decode solana log: %w", err)
	}
	return &Log{Signature: Signature(v.Value.Signature), Slot: Slot(v.Context.Slot), Messages: append([]string(nil), v.Value.Logs...), Failed: v.Value.Err != nil}, nil
}

// RecvState decodes one full account notification.
//
// Version:
//   - 2026-09-09: Added.
func (s *streamAccount) RecvState(ctx context.Context) (*AccountUpdate, error) {
	data, err := s.recv(ctx)
	if err != nil {
		return nil, err
	}
	var v sdkWS.AccountResult
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("failed to decode solana account: %w", err)
	}
	return decodeAccountUpdate(s.address, &v)
}

// Recv returns the next changed account slot.
//
// Version:
//   - 2026-09-09: Added.
func (s *streamAccount) Recv(ctx context.Context) (Slot, error) {
	v, err := s.RecvState(ctx)
	if err != nil {
		return 0, err
	}
	return v.Slot, nil
}
