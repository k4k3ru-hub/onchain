package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip delegates to the injected transport.
//
// Version:
//   - 2026-09-20: Added.
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func address(t *testing.T) sui.Address {
	t.Helper()
	a, err := sui.ParseAddress("0x1")
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func fakeClient(t *testing.T, f roundTripFunc) *Client {
	t.Helper()
	c, err := NewClient(Config{HTTPClient: &http.Client{Transport: f}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestCompositionAndPrecision verifies the operation graph, query and exact numeric decoding.
//
// Version:
//   - 2026-09-20: Added.
func TestCompositionAndPrecision(t *testing.T) {
	calls := 0
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/pools/v2" || r.URL.Query().Get("poolId") != address(t).String() || r.Header.Get("Accept") != "application/json" {
			t.Fatal("request mismatch")
		}
		return response(`{"pool_id":"0x1","liquidity_usd":9007199254740993.00000000001,"fee":"500","apr":127.8915466502802,"reward_apr":0,"reward_infos":[{"vault_coin_type":"2::sui::SUI","emissions_per_second":"109802030489467260848046080"}],"unlocked":true}`), nil
	})
	if c.Pools == nil || calls != 0 {
		t.Fatal("composition performed IO")
	}
	p, err := c.Pools.Get(t.Context(), address(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.LiquidityUSD.String() != "9007199254740993.00000000001" || p.APR.String() != "127.8915466502802" || p.RewardAPR.String() != "0" || p.APR7d != nil || p.RewardInfos[0].EmissionsPerSecond.String() != "109802030489467260848046080" {
		t.Fatalf("precision: %+v", p)
	}
	if calls != 1 {
		t.Fatal("unexpected retries")
	}
}

// TestMissingAndInvalidResponses distinguishes null arrays and rejects invalid payloads.
//
// Version:
//   - 2026-09-20: Added.
func TestMissingAndInvalidResponses(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[]`, `{"pool_id":"0x2"}`, `{"pool_id":"0x1","apr":"NaN"}`, `{"pool_id":"0x1"} {}`, `<html>challenge</html>`} {
		c := fakeClient(t, func(*http.Request) (*http.Response, error) { return response(body), nil })
		if _, err := c.Pools.Get(t.Context(), address(t)); err == nil {
			t.Fatalf("accepted: %s", body)
		}
	}
	for _, tc := range []struct {
		body       string
		nilRewards bool
	}{
		{`{"pool_id":"0x1"}`, true}, {`{"pool_id":"0x1","reward_infos":null}`, true}, {`{"pool_id":"0x1","reward_infos":[]}`, false},
	} {
		c := fakeClient(t, func(*http.Request) (*http.Response, error) { return response(tc.body), nil })
		p, err := c.Pools.Get(t.Context(), address(t))
		if err != nil || (p.RewardInfos == nil) != tc.nilRewards {
			t.Fatalf("missing distinction: %v %v", p, err)
		}
	}
}

// TestHTTPAndTransportErrors preserves error inspection without leaking response bodies.
//
// Version:
//   - 2026-09-20: Added.
func TestHTTPAndTransportErrors(t *testing.T) {
	for _, code := range []int{404, 429, 500} {
		c := fakeClient(t, func(*http.Request) (*http.Response, error) {
			r := response("confidential")
			r.StatusCode = code
			r.Header.Set("Retry-After", "60")
			return r, nil
		})
		_, err := c.Pools.Get(t.Context(), address(t))
		var status *HTTPError
		if !errors.As(err, &status) || status.StatusCode != code || status.RetryAfter != "60" || strings.Contains(err.Error(), "confidential") {
			t.Fatalf("HTTP error: %v", err)
		}
	}
	sentinel := errors.New("confidential transport")
	c := fakeClient(t, func(*http.Request) (*http.Response, error) { return nil, sentinel })
	_, err := c.Pools.Get(t.Context(), address(t))
	if !errors.Is(err, sentinel) || strings.Contains(err.Error(), "confidential") {
		t.Fatalf("transport: %v", err)
	}
	c = fakeClient(t, func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = c.Pools.Get(ctx, address(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

// TestGuardsAndResponseLimit validates inputs before IO and bounds response size.
//
// Version:
//   - 2026-09-20: Added.
func TestGuardsAndResponseLimit(t *testing.T) {
	for _, base := range []string{"file:///tmp/a", "https://user:secret@example.com", "https://example.com?secret=x"} {
		if _, err := NewClient(Config{BaseURL: base}); err == nil {
			t.Fatal("invalid URL accepted")
		}
	}
	c := fakeClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected IO"); return nil, nil })
	if _, err := c.Pools.Get(nil, address(t)); err == nil {
		t.Fatal("nil context")
	}
	if _, err := c.Pools.Get(t.Context(), sui.Address{}); err == nil {
		t.Fatal("zero address")
	}
	var nilPools *PoolsClient
	if _, err := nilPools.Get(t.Context(), address(t)); err == nil {
		t.Fatal("nil client")
	}
	c = fakeClient(t, func(*http.Request) (*http.Response, error) {
		return response(strings.Repeat(" ", maxResponseBytes+1)), nil
	})
	if _, err := c.Pools.Get(t.Context(), address(t)); err == nil || !strings.Contains(err.Error(), "too_long") {
		t.Fatalf("limit: %v", err)
	}
}
