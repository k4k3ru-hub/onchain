package solana

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestStreamLiveness verifies data-only, pong-only and silent-peer behavior.
//
// Version:
//   - 2026-09-09: Added.
func TestStreamLiveness(t *testing.T) {
	for _, mode := range []string{"data", "pong", "silent", "rpc_error"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer func() {
					if err := conn.Close(); err != nil {
						t.Error(err)
					}
				}()
				if mode != "pong" {
					conn.SetPingHandler(func(string) error { return nil })
				}
				var req struct {
					ID uint32 `json:"id"`
				}
				if err := conn.ReadJSON(&req); err != nil {
					t.Error(err)
					return
				}
				if mode == "rpc_error" {
					if err := conn.WriteJSON(map[string]any{"id": req.ID, "error": map[string]any{"code": -32005, "message": "not logged"}}); err != nil {
						t.Error(err)
					}
				} else if err := conn.WriteJSON(map[string]any{"id": req.ID, "result": 1}); err != nil {
					t.Error(err)
					return
				}
				stopped := make(chan struct{})
				go func() {
					defer close(stopped)
					for {
						if _, _, err := conn.ReadMessage(); err != nil {
							return
						}
					}
				}()
				ticker := time.NewTicker(20 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-stopped:
						return
					case <-ticker.C:
						if mode == "data" {
							if err := conn.WriteJSON(map[string]any{"params": map[string]any{"subscription": 1, "result": map[string]any{"context": map[string]any{"slot": 42}, "value": map[string]any{"signature": strings.Repeat("1", 64), "logs": []string{}, "err": nil}}}}); err != nil {
								return
							}
						}
					}
				}
			}))
			defer server.Close()
			c, err := dialStreamWS(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), 300*time.Millisecond, 50*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			sub, err := c.subscribeLogs(Address{1}, CommitmentConfirmed)
			if mode == "rpc_error" {
				if err == nil || !strings.Contains(err.Error(), "-32005") {
					t.Fatalf("lost RPC error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Unsubscribe()
			ctx, cancel := context.WithTimeout(context.Background(), 550*time.Millisecond)
			defer cancel()
			if mode == "data" {
				for ctx.Err() == nil {
					_, err := sub.Recv(ctx)
					if err != nil && ctx.Err() == nil {
						t.Fatal(err)
					}
				}
			} else {
				_, err := sub.Recv(ctx)
				if mode == "silent" {
					if err == nil || ctx.Err() != nil {
						t.Fatalf("silent peer did not disconnect: %v", err)
					}
				} else if ctx.Err() == nil {
					t.Fatalf("healthy quiet peer disconnected: %v", err)
				}
			}
		})
	}
}

// TestStreamQueueOverflowFailsSession verifies that dropped deltas force recovery.
//
// Version:
//   - 2026-09-09: Added.
func TestStreamQueueOverflowFailsSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Error(err)
			}
		}()
		var req struct {
			ID uint32 `json:"id"`
		}
		if err := conn.ReadJSON(&req); err != nil {
			t.Error(err)
			return
		}
		if err := conn.WriteJSON(map[string]any{"id": req.ID, "result": 1}); err != nil {
			t.Error(err)
			return
		}
		for i := 0; i < 300; i++ {
			if err := conn.WriteJSON(map[string]any{"params": map[string]any{"subscription": 1, "result": json.RawMessage(`{}`)}}); err != nil {
				return
			}
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	c, err := dialStreamWS(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.subscribeLogs(Address{1}, CommitmentConfirmed); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.done:
		if !strings.Contains(c.failure().Error(), "queue=too_long") {
			t.Fatal(c.failure())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("overflow did not close connection")
	}
}
