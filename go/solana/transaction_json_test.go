package solana

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestJSONTransactions preserves the consumed fields for every supported version.
//
// Version:
//   - 2026-09-17: Added.
func TestJSONTransactions(t *testing.T) {
	for _, version := range []any{"legacy", 0, 1} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			var signature Signature
			signature[0] = 1
			var static, writable, readonly sdk.PublicKey
			static[0] = 1
			writable[0] = 2
			readonly[0] = 3
			loaded := map[string]any{"writable": []string{}, "readonly": []string{}}
			index := 0
			if version == 0 {
				loaded["writable"] = []string{writable.String()}
				loaded["readonly"] = []string{readonly.String()}
				index = 2
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var q struct {
					ID     json.RawMessage   `json:"id"`
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Error(err)
					return
				}
				var opts map[string]any
				if err := json.Unmarshal(q.Params[1], &opts); err != nil {
					t.Error(err)
					return
				}
				if q.Method != "getTransaction" || opts["encoding"] != "json" || opts["maxSupportedTransactionVersion"] != float64(1) {
					t.Error("incorrect request options")
				}
				balance := map[string]any{"accountIndex": index, "mint": static.String(), "owner": writable.String(), "uiTokenAmount": map[string]any{"amount": "12345678901234567890", "decimals": 6, "uiAmount": nil, "uiAmountString": "12345678901234.567890"}}
				message := map[string]any{"accountKeys": []string{static.String()}}
				if version == 1 {
					message["transactionConfig"] = map[string]any{"computeUnitLimit": 30000}
				}
				result := map[string]any{"slot": 42, "blockTime": 100, "version": version, "transaction": map[string]any{"signatures": []string{signature.String()}, "message": message}, "meta": map[string]any{"err": nil, "fee": 5000, "logMessages": []string{"Log truncated"}, "loadedAddresses": loaded, "preTokenBalances": []any{balance}, "postTokenBalances": []any{balance}}}
				if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": result}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client, err := NewRPCClient(context.Background(), RPCConfig{URL: server.URL, Commitment: CommitmentConfirmed})
			if err != nil {
				t.Fatal(err)
			}
			tx, err := client.Transaction(context.Background(), signature)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || tx.Slot != 42 || tx.Fee != 5000 || tx.Timestamp.Unix() != 100 || len(tx.Logs) != 1 || tx.Failed {
				t.Fatalf("tx=%+v calls=%d", tx, calls)
			}
			if tx.PreTokenBalances[0].Amount != "12345678901234567890" || int(tx.PreTokenBalances[0].AccountIndex) != index {
				t.Fatal("balance corrupted")
			}
			if version == 0 {
				if len(tx.AccountKeys) != 3 || tx.AccountKeys[1].String() != writable.String() || tx.AccountKeys[2].String() != readonly.String() {
					t.Fatal("loaded key order invalid")
				}
			} else if len(tx.AccountKeys) != 1 {
				t.Fatal("unexpected keys")
			}
		})
	}
}

// TestPermanentTransactionError preserves classification through wrapped errors.
//
// Version:
//   - 2026-09-17: Added.
func TestPermanentTransactionError(t *testing.T) {
	for _, code := range []int{-32015, -32600, -32601, -32602, 429, -32005} {
		err := fmt.Errorf("failed to fetch: %w", &jsonrpc.RPCError{Code: code, Message: "failure"})
		want := code == -32015 || code == -32600 || code == -32601 || code == -32602
		if IsPermanentTransactionError(err) != want {
			t.Fatalf("code=%d", code)
		}
	}
	if IsPermanentTransactionError(errors.New("timeout")) {
		t.Fatal("transient error classified permanent")
	}
}
