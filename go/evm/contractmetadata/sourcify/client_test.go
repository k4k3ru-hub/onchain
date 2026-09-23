package sourcify

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"io"
	"net/http"
	"strings"
	"testing"
)

type httpFunc func(*http.Request) (*http.Response, error)

// Do returns the injected HTTP response fixture.
//
// Version:
//   - 2026-09-23: Added.
func (f httpFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

// TestDeployment verifies bounded lookup and deployment response identity.
//
// Version:
//   - 2026-09-23: Added.
func TestDeployment(t *testing.T) {
	a := common.HexToAddress("0x1234")
	h := common.HexToHash("0x5678")
	for _, mode := range []string{"valid", "missing", "wrong_chain", "wrong_address", "bad_hash", "too_large", "http_error"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c, err := NewClient(httpFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Query().Get("fields") != "deployment" || r.Method != "GET" {
					t.Fatal("wrong request")
				}
				if _, ok := r.Context().Deadline(); !ok {
					t.Fatal("missing timeout")
				}
				chain, address, hash := "8453", a.Hex(), h.Hex()
				status := 200
				switch mode {
				case "wrong_chain":
					chain = "1"
				case "wrong_address":
					address = common.HexToAddress("0x9999").Hex()
				case "bad_hash":
					hash = "invalid"
				case "missing":
					hash = ""
				case "http_error":
					status = 429
				}
				body := fmt.Sprintf(`{"chainId":%q,"address":%q,"deployment":{"transactionHash":%q}}`, chain, address, hash)
				if mode == "too_large" {
					body = strings.Repeat("x", (64<<10)+1)
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			}), "https://example.invalid/server")
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Deployment(context.Background(), 8453, a)
			if calls != 1 {
				t.Fatalf("unexpected retry: %d", calls)
			}
			if mode == "valid" {
				if err != nil || result.TransactionHash != h {
					t.Fatalf("%+v %v", result, err)
				}
			} else if mode == "missing" {
				if err != nil || result.TransactionHash != (common.Hash{}) {
					t.Fatalf("%+v %v", result, err)
				}
			} else if err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}
