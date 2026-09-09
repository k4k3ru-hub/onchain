package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/solana"
)

type roundTrip func(*http.Request) (*http.Response, error)

// RoundTrip delegates to the injected test response.
//
// Version:
//   - 2026-09-09: Added.
func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestPoolStatistics verifies composition, precision and separate source windows.
//
// Version:
//   - 2026-09-09: Added.
func TestPoolStatistics(t *testing.T) {
	pool, err := solana.ParseAddress("3ucNos4NbumPLZNWztqGHNFFgkHeRMBQAVemeeomsUxv")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	c, err := NewClient(Config{HTTPClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/pools/info/ids" || r.URL.Query().Get("ids") != pool.String() {
			t.Fatalf("unexpected request: %s", r.URL)
		}
		body := `{"success":true,"data":[{"id":"` + pool.String() + `","tvl":9007199254740993.123,"day":{"feeApr":0,"apr":5,"rewardApr":[5,null]},"week":{"feeApr":1},"rewardDefaultInfos":[{"mint":{"address":"mint","decimals":9},"perSecond":0.1,"startTime":1,"endTime":2}]}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}})
	if err != nil || c.Pools == nil || calls != 0 {
		t.Fatalf("composition: %v", err)
	}
	p, err := c.Pools.Get(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || p.TVL.String() != "9007199254740993.123" || p.Day.FeeAPR.String() != "0" || p.Week.FeeAPR.String() != "1" || p.Month != nil || p.Day.RewardAPR[1] != nil || p.RewardDefaultInfos[0].EndTime == nil {
		t.Fatalf("incorrect statistics: %+v", p)
	}
}

// TestPoolFailures verifies missing data, malformed responses and rate-limit errors.
//
// Version:
//   - 2026-09-09: Added.
func TestPoolFailures(t *testing.T) {
	pool := solana.Address{1}
	for _, body := range []string{`null`, `{}`, `{"data":[]}`, `{"data":[null]}`, `{"data":[{"id":"wrong"}]}`, `{"success":false,"data":[]}`, `{"data":[{"id":"` + pool.String() + `","tvl":"NaN"}]}`, `<html>`} {
		c, err := NewClient(Config{HTTPClient: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Pools.Get(context.Background(), pool); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	calls := 0
	c, err := NewClient(Config{HTTPClient: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"30"}}, Body: io.NopCloser(strings.NewReader("confidential"))}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Pools.Get(context.Background(), pool)
	var status *HTTPError
	if !errors.As(err, &status) || status.StatusCode != 429 || status.RetryAfter != "30" || calls != 1 || strings.Contains(err.Error(), "confidential") {
		t.Fatalf("rate limit: %v", err)
	}
}
