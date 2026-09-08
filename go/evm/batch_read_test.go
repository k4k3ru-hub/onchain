package evm

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBatchReadCompositionAndOrdering verifies or implements the injected batch test boundary.
//
// Version:
//   - 2026-09-08: Added.
func TestBatchReadCompositionAndOrdering(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var in []struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if len(in) != 3 {
			t.Errorf("batch length %d", len(in))
		}
		out := make([]map[string]any, 0, len(in))
		for i := len(in) - 1; i >= 0; i-- {
			if in[i].Method != "eth_call" || string(in[i].Params[1]) != `"0x64"` {
				t.Errorf("wrong request: %+v", in[i])
			}
			v := map[string]any{"jsonrpc": "2.0", "id": in[i].ID, "result": "0x01"}
			if i == 1 {
				delete(v, "result")
				v["error"] = map[string]any{"code": 429, "message": "limited"}
			}
			out = append(out, v)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	c, err := NewHTTPClient(context.Background(), HTTPConfig{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	result, failures, err := c.ReadContracts(nil, common.HexToAddress("01"), [][]byte{{1}, {2}, {3}}, 100)
	if err != nil || requests != 1 || len(result) != 3 || len(failures) != 3 {
		t.Fatalf("batch failed: %v requests=%d", err, requests)
	}
	var rpcErr rpc.Error
	if failures[0] != nil || failures[2] != nil || !errors.As(failures[1], &rpcErr) || rpcErr.ErrorCode() != 429 || len(result[0]) != 1 || result[0][0] != 1 {
		t.Fatalf("lost per-call results: %v", failures)
	}
}

type failedBatchCaller struct{ err error }

// BatchCallContext returns the injected transport failure.
//
// Version:
//   - 2026-09-08: Added.
func (f failedBatchCaller) BatchCallContext(context.Context, []rpc.BatchElem) error { return f.err }

// TestBatchReadTransportFailure verifies transport errors remain inspectable without partial results.
//
// Version:
//   - 2026-09-08: Added.
func TestBatchReadTransportFailure(t *testing.T) {
	failure := context.DeadlineExceeded
	c := &HTTPClient{batchCaller: failedBatchCaller{failure}}
	values, perCall, err := c.ReadContracts(context.Background(), common.HexToAddress("01"), [][]byte{{1}}, 0)
	if !errors.Is(err, failure) || values != nil || perCall != nil {
		t.Fatalf("lost transport failure: %v", err)
	}
	for _, data := range [][][]byte{nil, make([][]byte, 65)} {
		if _, _, err := c.ReadContracts(nil, common.HexToAddress("01"), data, 0); err == nil {
			t.Fatal("invalid batch accepted")
		}
	}
}
