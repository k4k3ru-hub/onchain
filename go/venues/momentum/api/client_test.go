package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements the injected test transport.
// RoundTrip delegates to the injected test response.
//
// Version:
//   - 2026-09-09: Added.
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeClient(t *testing.T, fn roundTripFunc) *Client {
	t.Helper()
	c, err := NewClient(Config{HTTPClient: &http.Client{Transport: fn}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func address(t *testing.T) sui.Address {
	t.Helper()
	a, err := sui.ParseAddress("0x1")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestCompositionAndPoolResponses checks composition and upstream response shapes.
//
// Version:
//   - 2026-09-09: Added.
func TestCompositionAndPoolResponses(t *testing.T) {
	calls := 0
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.Header.Get("Accept") != "application/json" {
			t.Fatal("unexpected request")
		}
		switch r.URL.Path {
		case "/pools/v3":
			return response(`{"data":[{"poolId":"0x1","tvl":"9007199254740993.00000000001","fees24h":0,"aprBreakdown":{"fee":"12.25","total":"14.5","rewards":[{"coinType":"reward","apr":"2.25","amountPerDay":1.125}]},"rewarders":[{"coin_type":"reward","flow_rate":0,"hasEnded":true}]}]}`), nil
		case "/pools/v3/" + address(t).String():
			return response(`{"data":{"poolId":"0x1","apy":"50"}}`), nil
		case "/pools/v3/rewarders-apy/" + address(t).String():
			return response(`{"pool_id":"0x1","apy":50,"rewarders":[]}`), nil
		default:
			return nil, fmt.Errorf("unexpected path")
		}
	})
	if c.Pools == nil || calls != 0 {
		t.Fatal("construction performed IO or omitted group")
	}
	pools, err := c.Pools.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := pools[0]
	if p.TVL.String() != "9007199254740993.00000000001" || p.Fees24h.String() != "0" || p.Volume24h != nil || p.APRBreakdown.Rewards[0].AmountPerDay.String() != "1.125" || !*p.Rewarders[0].HasEnded {
		t.Fatalf("lossy pool: %+v", p)
	}
	one, err := c.Pools.Get(context.Background(), address(t))
	if err != nil {
		t.Fatal(err)
	}
	if one.APRBreakdown != nil || one.LegacyAPY.String() != "50" {
		t.Fatal("legacy apy converted to apr")
	}
	rewards, err := c.Pools.RewardsAPY(context.Background(), address(t))
	if err != nil {
		t.Fatal(err)
	}
	if rewards.APY.String() != "50" || rewards.Rewarders == nil {
		t.Fatal("incorrect rewards")
	}
	if calls != 3 {
		t.Fatal("unexpected retries")
	}
}

// TestInvalidResponses rejects malformed and mismatched pool responses.
//
// Version:
//   - 2026-09-09: Added.
func TestInvalidResponses(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"data":null}`, `{"data":{"poolId":"0x2"}}`, `{"data":{"poolId":"0x1","tvl":"abc"}}`, `{"data":{"poolId":"0x1"}} {}`, `<html>challenge</html>`} {
		t.Run(body, func(t *testing.T) {
			c := fakeClient(t, func(*http.Request) (*http.Response, error) { return response(body), nil })
			if _, err := c.Pools.Get(context.Background(), address(t)); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
	c := fakeClient(t, func(*http.Request) (*http.Response, error) { return response(`{"data":[]}`), nil })
	got, err := c.Pools.List(context.Background())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list: %v %v", got, err)
	}
}

// TestHTTPFailureAndCancellation checks failure classification and cancellation.
//
// Version:
//   - 2026-09-09: Added.
func TestHTTPFailureAndCancellation(t *testing.T) {
	calls := 0
	c := fakeClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		r := response("confidential response")
		r.StatusCode = 429
		r.Header.Set("Retry-After", "60")
		return r, nil
	})
	_, err := c.Pools.List(context.Background())
	var status *HTTPError
	if !errors.As(err, &status) || status.StatusCode != 429 || status.RetryAfter != "60" || strings.Contains(err.Error(), "confidential") || calls != 1 {
		t.Fatalf("incorrect HTTP error: %v", err)
	}
	sentinel := errors.New("confidential transport url")
	c = fakeClient(t, func(*http.Request) (*http.Response, error) { return nil, sentinel })
	_, err = c.Pools.List(context.Background())
	if !errors.Is(err, sentinel) || strings.Contains(err.Error(), "confidential") {
		t.Fatalf("transport error leaked or lost: %v", err)
	}
	c = fakeClient(t, func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Pools.List(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// TestValidationAndMalformedNumbers checks validation before network access.
//
// Version:
//   - 2026-09-09: Added.
func TestValidationAndMalformedNumbers(t *testing.T) {
	for _, base := range []string{"file:///tmp/a", "https://user:secret@example.com", "https://example.com?token=secret"} {
		if _, err := NewClient(Config{BaseURL: base}); err == nil {
			t.Fatal("accepted invalid URL")
		}
	}
	c := fakeClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected IO"); return nil, nil })
	if _, err := c.Pools.List(nil); err == nil {
		t.Fatal("accepted nil context")
	}
	if _, err := c.Pools.Get(context.Background(), sui.Address{}); err == nil {
		t.Fatal("accepted zero pool")
	}
	var p Pool
	if err := json.Unmarshal([]byte(`{"tvl":"NaN"}`), &p); err == nil {
		t.Fatal("accepted NaN")
	}
}
