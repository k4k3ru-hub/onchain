package websocket

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
)

type defaultDialer struct {
	maxBytes int64
	timeout  time.Duration
}
type wireConnection struct {
	conn     *gorilla.Conn
	writeMu  sync.Mutex
	once     sync.Once
	closeErr error
}

// Dial opens a bounded connection using the existing module WebSocket dependency.
//
// Version:
//   - 2026-09-06: Added.
func (d *defaultDialer) Dial(ctx context.Context, endpoint string) (Connection, error) {
	dialer := gorilla.Dialer{HandshakeTimeout: d.timeout}
	conn, response, err := dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		if response != nil && response.Body != nil {
			err = errors.Join(err, response.Body.Close())
		}
		return nil, err
	}
	conn.SetReadLimit(d.maxBytes)
	return &wireConnection{conn: conn}, nil
}

// Read receives one message. Cancellation closes the connection to unblock I/O.
//
// Version:
//   - 2026-09-06: Added.
func (c *wireConnection) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	finish := c.cancelIO(ctx)
	kind, data, err := c.conn.ReadMessage()
	cancelErr := finish()
	if err != nil || cancelErr != nil {
		return nil, errors.Join(err, cancelErr)
	}
	if kind != gorilla.TextMessage {
		return nil, fmt.Errorf("failed to read lighter frame: message_type=invalid")
	}
	return data, nil
}

// Write serializes writes to the socket and closes it on cancellation.
//
// Version:
//   - 2026-09-06: Added.
func (c *wireConnection) Write(ctx context.Context, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	finish := c.cancelIO(ctx)
	err := c.conn.WriteMessage(gorilla.TextMessage, data)
	return errors.Join(err, finish())
}

// Ping sends a control frame satisfying the server keepalive requirement.
//
// Version:
//   - 2026-09-06: Added.
func (c *wireConnection) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	finish := c.cancelIO(ctx)
	err := c.conn.WriteControl(gorilla.PingMessage, nil, deadline)
	return errors.Join(err, finish())
}

// Close closes the underlying socket once and retains the close error.
//
// Version:
//   - 2026-09-06: Added.
func (c *wireConnection) Close() error {
	c.once.Do(func() { c.closeErr = c.conn.Close() })
	return c.closeErr
}
func (c *wireConnection) cancelIO(ctx context.Context) func() error {
	finished := make(chan struct{})
	var closeErr error
	stop := context.AfterFunc(ctx, func() { closeErr = c.Close(); close(finished) })
	return func() error {
		if !stop() {
			<-finished
			return errors.Join(ctx.Err(), closeErr)
		}
		return ctx.Err()
	}
}
