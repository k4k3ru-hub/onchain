package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip delegates to the injected test transport.
//
// Version:
//   - 2026-09-14: Added.
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func fakeClient(t *testing.T, f roundTripFunc) *Client {
	t.Helper()
	c, err := NewClient(Config{HTTPClient: &http.Client{Transport: f}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestCompositionAndCapturedStatistics verifies dependency wiring and mixed-unit preservation.
//
// Version:
//   - 2026-09-14: Added.
func TestCompositionAndCapturedStatistics(t *testing.T) {
	body, err := os.ReadFile("testdata/stats_pools.json")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodGet || r.Header.Get("Accept") != "application/json" || r.URL.String() != "https://example.com/prefix/v2/sui/stats_pools?limit=1&offset=2" {
			t.Fatalf("unexpected request: %v", r.URL)
		}
		return response(string(body)), nil
	})}
	c, err := NewClient(Config{BaseURL: "https://example.com/prefix/", HTTPClient: httpClient})
	if err != nil || c.Pools == nil || c.Pools.http != httpClient || calls != 0 {
		t.Fatalf("composition failed: %v", err)
	}
	page, err := c.Pools.List(context.Background(), ListParams{Limit: 1, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || *page.Total != 44019 || len(page.Pools) != 1 {
		t.Fatalf("unexpected page: %+v calls=%d", page, calls)
	}
	p := page.Pools[0]
	if p.Address != "0xb8d7d9e66a60c239e7a60110efcf8de6c705580ed924d0dde141f4a0e2c90105" || p.TotalAPR.String() != "0.3962404734165865" || p.APR.FeeAPR24h.String() != "0.2406720960157855" || len(p.RewarderAPR) != 5 || *p.RewarderAPR[0] != "0.41760996615576%" || *p.RewarderAPR[4] != "0%" {
		t.Fatalf("lost identity, units or reward slots: %+v", p)
	}
	if p.PureTVLInUSD.String() != "3795290.60235323119989193" || p.Fee24h.String() != "2550.0178381413166" || p.Object == nil || *p.Object.IsPause || *p.IsClosed || p.CoinA == nil || *p.CoinA.Decimals != 6 || p.CoinA.Address != p.CoinAAddress || len(p.Vaults) != 1 || string(p.StableFarming) != "null" {
		t.Fatalf("lost metadata: %+v", p)
	}
	var envelope struct {
		Fields struct {
			Rewarders       []json.RawMessage `json:"rewarders"`
			LastUpdatedTime int64             `json:"last_updated_time"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(p.Object.RewarderManager, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Fields.Rewarders) != 2 || envelope.Fields.LastUpdatedTime != 1789310813 {
		t.Fatal("reward manager was lost or conflated with APR slots")
	}
	defaults, err := NewClient(Config{})
	if err != nil || defaults.Pools.baseURL != MainnetURL || defaults.Pools.http.Timeout != 15*time.Second {
		t.Fatalf("default composition: %v", err)
	}
}

// TestMissingAndZeroMetrics verifies unavailable metrics and source totals remain distinct.
//
// Version:
//   - 2026-09-14: Added.
func TestMissingAndZeroMetrics(t *testing.T) {
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.RawQuery != "offset=0" {
			t.Fatal(r.URL)
		}
		return response(`{"code":0,"data":{"total":2,"lp_list":[{"address":"0x1","total_apr":"0","apr":{"fee_apr_24h":0},"fee_24_h":"9007199254740993.00000000001","rewarder_apr":[null,"0%"],"stable_farming":{"unknown":9007199254740993}},{"address":"0x2","total_apr":null,"apr":null}]}}`), nil
	})
	page, err := c.Pools.List(context.Background(), ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	a, b := page.Pools[0], page.Pools[1]
	if a.TotalAPR == nil || a.TotalAPR.String() != "0" || a.APR.FeeAPR24h.String() != "0" || a.Fee24h.String() != "9007199254740993.00000000001" || a.RewarderAPR[0] != nil || a.VolumeInUSD24h != nil || b.TotalAPR != nil || b.APR != nil || b.RewarderAPR != nil || b.IsClosed != nil || string(a.StableFarming) != `{"unknown":9007199254740993}` {
		t.Fatal("missing values, precision or raw farm metadata changed")
	}
	c = fakeClient(t, func(*http.Request) (*http.Response, error) {
		return response(`{"code":0,"data":{"total":0,"lp_list":[]}}`), nil
	})
	page, err = c.Pools.List(context.Background(), ListParams{})
	if err != nil || page.Pools == nil || len(page.Pools) != 0 {
		t.Fatalf("empty page: %v %v", page, err)
	}
}

// TestInvalidResponses rejects malformed envelopes, identities and numeric metrics.
//
// Version:
//   - 2026-09-14: Added.
func TestInvalidResponses(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `{"code":0}`, `{"code":0,"data":null}`,
		`{"data":{"total":0,"lp_list":[]}}`, `{"code":0,"data":{"lp_list":[]}}`,
		`{"code":0,"data":{"total":0,"lp_list":null}}`, `{"code":0,"data":{"total":-1,"lp_list":[]}}`,
		`{"code":0,"data":{"total":1,"lp_list":[null]}}`,
		`{"code":0,"data":{"total":1,"lp_list":[{"address":"0x0"}]}}`,
		`{"code":0,"data":{"total":1,"lp_list":[{"address":"invalid"}]}}`,
		`{"code":0,"data":{"total":1,"lp_list":[{"address":"0x1","total_apr":"NaN"}]}}`,
		`{"code":0,"data":{"total":1,"lp_list":[{"address":"0x1","apr":{"fee_apr_24h":"12%"}}]}}`,
		`{"code":0,"data":{"total":0,"lp_list":[]}} {}`, `<html>challenge</html>`,
	} {
		t.Run(body, func(t *testing.T) {
			c := fakeClient(t, func(*http.Request) (*http.Response, error) { return response(body), nil })
			if _, err := c.Pools.List(context.Background(), ListParams{}); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}

// TestFailures verifies typed failures, error chains, cancellation and no retries.
//
// Version:
//   - 2026-09-14: Added.
func TestFailures(t *testing.T) {
	calls := 0
	c := fakeClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		r := response("confidential body")
		r.StatusCode = 429
		r.Header.Set("Retry-After", "60")
		return r, nil
	})
	_, err := c.Pools.List(context.Background(), ListParams{})
	var status *HTTPError
	if !errors.As(err, &status) || status.StatusCode != 429 || status.RetryAfter != "60" || calls != 1 || strings.Contains(err.Error(), "confidential") {
		t.Fatalf("HTTP error: %v", err)
	}
	c = fakeClient(t, func(*http.Request) (*http.Response, error) {
		return response(`{"code":123,"msg":"confidential","data":null}`), nil
	})
	_, err = c.Pools.List(context.Background(), ListParams{})
	var upstream *APIError
	if !errors.As(err, &upstream) || upstream.Code != 123 || strings.Contains(err.Error(), "confidential") {
		t.Fatalf("API error: %v", err)
	}
	sentinel := errors.New("confidential transport url")
	c = fakeClient(t, func(*http.Request) (*http.Response, error) { return nil, sentinel })
	_, err = c.Pools.List(context.Background(), ListParams{})
	if !errors.Is(err, sentinel) || strings.Contains(err.Error(), "confidential") {
		t.Fatalf("transport error: %v", err)
	}
	c = fakeClient(t, func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Pools.List(ctx, ListParams{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c = fakeClient(t, func(*http.Request) (*http.Response, error) { return response(`{"code":"confidential"}`), nil })
	_, err = c.Pools.List(context.Background(), ListParams{})
	var decode *json.UnmarshalTypeError
	if !errors.As(err, &decode) || strings.Contains(err.Error(), "confidential") {
		t.Fatalf("decode error: %v", err)
	}
}

// TestValidation rejects invalid configuration and requests before network access.
//
// Version:
//   - 2026-09-14: Added.
func TestValidation(t *testing.T) {
	for _, base := range []string{"file:///tmp/a", "https://user:secret@example.com", "https://example.com?token=secret", "https://example.com/#fragment", "://"} {
		if _, err := NewClient(Config{BaseURL: base}); err == nil {
			t.Fatal("accepted invalid URL")
		}
	}
	c := fakeClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected IO"); return nil, nil })
	for _, params := range []ListParams{{Limit: -1}, {Offset: -1}} {
		if _, err := c.Pools.List(context.Background(), params); err == nil {
			t.Fatal("accepted invalid pagination")
		}
	}
	if _, err := c.Pools.List(nil, ListParams{}); err == nil {
		t.Fatal("accepted nil context")
	}
	var pools *PoolsClient
	if _, err := pools.List(context.Background(), ListParams{}); err == nil {
		t.Fatal("accepted nil client")
	}
}

type failingBody struct{ readErr, closeErr error }

// Read returns the injected body read error.
//
// Version:
//   - 2026-09-14: Added.
func (b failingBody) Read([]byte) (int, error) { return 0, b.readErr }

// Close returns the injected body close error.
//
// Version:
//   - 2026-09-14: Added.
func (b failingBody) Close() error { return b.closeErr }

// TestBodyFailures checks size limits and preserves read and close failure chains.
//
// Version:
//   - 2026-09-14: Added.
func TestBodyFailures(t *testing.T) {
	c := fakeClient(t, func(*http.Request) (*http.Response, error) {
		return response(strings.Repeat(" ", maxResponseBytes+1)), nil
	})
	if _, err := c.Pools.List(context.Background(), ListParams{}); err == nil || !strings.Contains(err.Error(), "too_long") {
		t.Fatalf("size limit: %v", err)
	}
	readErr, closeErr := errors.New("confidential read"), errors.New("confidential close")
	c = fakeClient(t, func(*http.Request) (*http.Response, error) {
		r := response("")
		r.Body = failingBody{readErr, closeErr}
		return r, nil
	})
	_, err := c.Pools.List(context.Background(), ListParams{})
	if !errors.Is(err, readErr) || !errors.Is(err, closeErr) || strings.Contains(err.Error(), "confidential") {
		t.Fatalf("body errors: %v", err)
	}
}
