package solana

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWSClientUsesGatewayCompatibleRequestIDs(t *testing.T) {
	problems := make(chan error, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			problems <- err
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				problems <- err
			}
		}()
		for i := 0; i < 2; i++ {
			var req struct {
				ID     uint64 `json:"id"`
				Method string `json:"method"`
			}
			if err := conn.ReadJSON(&req); err != nil {
				problems <- err
				return
			}
			if req.ID > 1<<31-1 {
				problems <- fmt.Errorf("request ID exceeds gateway range")
				return
			}
			if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": i + 1}); err != nil {
				problems <- err
				return
			}
			method := "logsNotification"
			value := json.RawMessage(`{"signature":"` + strings.Repeat("1", 64) + `","err":null,"logs":[]}`)
			if req.Method == "accountSubscribe" {
				method = "accountNotification"
				value = json.RawMessage(`{"lamports":1,"owner":"11111111111111111111111111111111","data":["","base64"],"executable":false,"rentEpoch":0}`)
			}
			if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "method": method, "params": map[string]any{"subscription": i + 1, "result": map[string]any{"context": map[string]any{"slot": 42}, "value": value}}}); err != nil {
				problems <- err
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := NewWSClient(ctx, WSConfig{URL: "ws" + strings.TrimPrefix(server.URL, "http"), Commitment: CommitmentConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	logs, err := c.SubscribeLogs(Address{1})
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	if value, err := logs.Recv(ctx); err != nil || value == nil || value.Slot != 42 {
		t.Fatalf("log notification: value=%v err=%v", value, err)
	}
	accounts, err := c.SubscribeAccountChanges(Address{2})
	if err != nil {
		t.Fatal(err)
	}
	defer accounts.Close()
	if slot, err := accounts.Recv(ctx); err != nil || slot != 42 {
		t.Fatalf("account notification: slot=%d err=%v", slot, err)
	}
	select {
	case err := <-problems:
		t.Fatal(err)
	default:
	}
}
