package sourcify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
	"github.com/k4k3ru-hub/onchain/go/evm/erc20/tax"
)

type fakeHTTP struct {
	data     string
	status   int
	err      error
	requests []*http.Request
	body     *trackedBody
}
type trackedBody struct {
	io.Reader
	closed bool
}

// Close tracks closure of source responses.
//
// Version:
//   - 2026-09-22: Added.
func (b *trackedBody) Close() error { b.closed = true; return nil }

// Do captures source requests without accessing a network.
//
// Version:
//   - 2026-09-22: Added.
func (f *fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	f.body = &trackedBody{Reader: strings.NewReader(f.data)}
	return &http.Response{StatusCode: f.status, Body: f.body}, nil
}

func sourceFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../testdata/sourcify.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func sourceRequest() tax.SourceRequest {
	return tax.SourceRequest{ChainID: evm.ChainIDBaseMainnet, Token: common.HexToAddress("0xaaeb2b5aff0bcb7fbed2f8917725196ee67eee7e")}
}

// TestSourceUsesBoundedReadOnlyV2Request verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestSourceUsesBoundedReadOnlyV2Request(t *testing.T) {
	h := &fakeHTTP{data: sourceFixture(t), status: 200}
	c, err := NewClient(h, "https://sourcify.dev/server/")
	if err != nil {
		t.Fatal(err)
	}
	if c.http != h {
		t.Fatal("http dependency not composed")
	}
	bundle, err := c.Source(context.Background(), sourceRequest())
	if err != nil {
		t.Fatal(err)
	}
	if bundle.CompilerVersion != "0.8.37+commit.f401782d" || bundle.ContractFile != "ShinyLIMPET.sol" || bundle.ContractName != "ShinyLIMPET" || len(bundle.Input) == 0 {
		t.Fatalf("unexpected bundle: %+v", bundle)
	}
	if len(h.requests) != 1 || h.requests[0].Method != "GET" || h.requests[0].URL.Path != "/server/v2/contract/8453/"+sourceRequest().Token.Hex() || h.requests[0].URL.Query().Get("fields") != "compilation,stdJsonInput" || !h.body.closed {
		t.Fatal("unexpected acquisition or response lifecycle")
	}
	if _, ok := h.requests[0].Context().Deadline(); !ok {
		t.Fatal("missing request deadline")
	}
}

// TestSourceLeavesRetriesToCaller verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestSourceLeavesRetriesToCaller(t *testing.T) {
	for _, status := range []int{400, 404, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			h := &fakeHTTP{status: status, data: "confidential-response"}
			c, err := NewClient(h, "https://sourcify.dev/server")
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Source(context.Background(), sourceRequest())
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != status || strings.Contains(err.Error(), h.data) || len(h.requests) != 1 || !h.body.closed {
				t.Fatalf("unexpected HTTP failure: %v", err)
			}
		})
	}
	failure := errors.New("transport failure")
	h := &fakeHTTP{err: failure}
	c, err := NewClient(h, "https://sourcify.dev/server")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Source(context.Background(), sourceRequest()); !errors.Is(err, failure) {
		t.Fatal("transport error lost")
	}
}

// TestSourceRejectsInvalidResponse verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestSourceRejectsInvalidResponse(t *testing.T) {
	for _, name := range []string{"identity", "compiler", "qualified name", "missing input", "oversized", "invalid json"} {
		t.Run(name, func(t *testing.T) {
			var response map[string]any
			if err := json.Unmarshal([]byte(sourceFixture(t)), &response); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "identity":
				response["chainId"] = "1"
			case "compiler":
				response["compilation"].(map[string]any)["compiler"] = "vyper"
			case "qualified name":
				response["compilation"].(map[string]any)["fullyQualifiedName"] = "invalid"
			case "missing input":
				delete(response, "stdJsonInput")
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			h := &fakeHTTP{status: 200, data: string(encoded)}
			if name == "oversized" {
				h.data = strings.Repeat(" ", 4*1024*1024+1)
			}
			if name == "invalid json" {
				h.data = "{"
			}
			c, err := NewClient(h, "https://sourcify.dev/server")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.Source(context.Background(), sourceRequest()); err == nil || !h.body.closed {
				t.Fatal("invalid source response accepted")
			}
		})
	}
}
