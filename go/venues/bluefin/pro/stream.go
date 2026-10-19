package pro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Connection owns one transport; implementations must handle WebSocket ping/pong and context cancellation.
type Connection interface {
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
	Close() error
}
type Dialer interface {
	Dial(context.Context, string, http.Header) (Connection, error)
}
type StreamClient struct {
	endpoints Endpoints
	dialer    Dialer
}
type Subscription struct {
	connection Connection
	buffer     []Event
	once       sync.Once
	closeErr   error
}
type Event struct {
	Type    string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}
type PartialDepth struct {
	Symbol      string     `json:"symbol"`
	Bids        [][]string `json:"bidsE9"`
	Asks        [][]string `json:"asksE9"`
	UpdateID    uint64     `json:"orderbookUpdateId"`
	UpdatedAtMS int64      `json:"updatedAtMillis"`
	DepthLevel  string     `json:"depthLevel"`
}

// NewStreamClient composes a separate stream client with an explicitly owned socket transport.
//
// Version:
//   - 2026-10-14: Added.
func NewStreamClient(network Network, dialer Dialer) (*StreamClient, error) {
	endpoints, err := EndpointsFor(network)
	if err != nil {
		return nil, err
	}
	if dialer == nil {
		return nil, fmt.Errorf("failed to create bluefin stream client: dialer=null")
	}
	return &StreamClient{endpoints: endpoints, dialer: dialer}, nil
}

// Market subscribes to SUI full partial-depth snapshots, recent trades and ticker observations.
// It returns only after a successful subscription acknowledgment.
//
// Version:
//   - 2026-10-14: Added.
func (c *StreamClient) Market(ctx context.Context) (*Subscription, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to subscribe bluefin market: client=null")
	}
	payload := struct {
		Method  string `json:"method"`
		Streams []struct {
			Symbol  string   `json:"symbol"`
			Streams []string `json:"streams"`
		} `json:"dataStreams"`
	}{Method: "Subscribe"}
	payload.Streams = append(payload.Streams, struct {
		Symbol  string   `json:"symbol"`
		Streams []string `json:"streams"`
	}{"SUI-PERP", []string{"Partial_Depth_20", "Recent_Trade", "Ticker"}})
	return c.subscribe(ctx, c.endpoints.MarketStream, "", payload)
}

// Account subscribes to private order, fill, position, balance and command-failure observations.
// A live connection may outlast the token; reconnecting requires a currently valid token.
//
// Version:
//   - 2026-10-14: Added.
func (c *StreamClient) Account(ctx context.Context, token string) (*Subscription, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to subscribe bluefin account: client=null")
	}
	if token == "" || len(token) > 32<<10 || strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("failed to subscribe bluefin account: token=invalid")
	}
	payload := struct {
		Method  string   `json:"method"`
		Streams []string `json:"dataStreams"`
	}{"Subscribe", []string{"AccountOrderUpdate", "AccountTradeUpdate", "AccountPositionUpdate", "AccountUpdate", "AccountCommandFailureUpdate"}}
	return c.subscribe(ctx, c.endpoints.AccountStream, token, payload)
}

func (c *StreamClient) subscribe(ctx context.Context, endpoint, token string, payload any) (subscription *Subscription, resultErr error) {
	if ctx == nil || c == nil {
		return nil, fmt.Errorf("failed to subscribe bluefin stream: dependency=null")
	}
	header := make(http.Header)
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	connection, err := c.dialer.Dial(ctx, endpoint, header)
	if err != nil {
		return nil, &requestError{err}
	}
	if connection == nil {
		return nil, fmt.Errorf("failed to subscribe bluefin stream: connection=null")
	}
	defer func() {
		if resultErr != nil {
			if err := connection.Close(); err != nil {
				resultErr = errors.Join(resultErr, &requestError{err})
			}
		}
	}()
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, &requestError{err}
	}
	if err := connection.Write(ctx, data); err != nil {
		return nil, &requestError{err}
	}
	s := &Subscription{connection: connection}
	for len(s.buffer) < 256 {
		data, err := connection.Read(ctx)
		if err != nil {
			return nil, &requestError{err}
		}
		if len(data) > 4<<20 {
			return nil, fmt.Errorf("failed to subscribe bluefin stream: message=too_long")
		}
		var ack struct {
			Success *bool `json:"success"`
		}
		if err := json.Unmarshal(data, &ack); err != nil {
			return nil, &requestError{err}
		}
		if ack.Success != nil {
			if !*ack.Success {
				return nil, fmt.Errorf("failed to subscribe bluefin stream: subscription=rejected")
			}
			return s, nil
		}
		event, err := parseEvent(data)
		if err != nil {
			return nil, err
		}
		s.buffer = append(s.buffer, event)
	}
	return nil, fmt.Errorf("failed to subscribe bluefin stream: acknowledgment=too_late")
}

func parseEvent(data []byte) (Event, error) {
	if len(data) > 4<<20 {
		return Event{}, fmt.Errorf("failed to parse bluefin event: message=too_long")
	}
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return Event{}, &requestError{err}
	}
	if e.Type == "" || len(e.Type) > 64 || len(e.Payload) == 0 || string(e.Payload) == "null" {
		return Event{}, fmt.Errorf("failed to parse bluefin event: event=invalid")
	}
	return e, nil
}

// Next reads one event in order, including observations buffered before the acknowledgment.
//
// Version:
//   - 2026-10-14: Added.
func (s *Subscription) Next(ctx context.Context) (Event, error) {
	if s == nil || s.connection == nil || ctx == nil {
		return Event{}, fmt.Errorf("failed to read bluefin stream: dependency=null")
	}
	if err := ctx.Err(); err != nil {
		return Event{}, &requestError{err}
	}
	if len(s.buffer) > 0 {
		e := s.buffer[0]
		s.buffer = s.buffer[1:]
		return e, nil
	}
	data, err := s.connection.Read(ctx)
	if err != nil {
		return Event{}, &requestError{err}
	}
	return parseEvent(data)
}

// Close releases the owned socket without discarding persisted order evidence.
//
// Version:
//   - 2026-10-14: Added.
func (s *Subscription) Close() error {
	if s == nil || s.connection == nil {
		return nil
	}
	s.once.Do(func() {
		if err := s.connection.Close(); err != nil {
			s.closeErr = &requestError{err}
		}
	})
	return s.closeErr
}
