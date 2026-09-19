package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClientCompositionAndList verifies composition, exact decimals and source nulls.
//
// Version:
//   - 2026-09-19: Added.
func TestClientCompositionAndList(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/pools/info" || r.URL.Query().Get("page") != "2" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		if _, err := io.WriteString(w, `[{"address":"0x1","day":{"apr":{"feeApr":"155.1234567890123456789","rewardApr":0,"total":null},"volume":"42"},"totalApr":"999"}]`); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil || client.Pools == nil || calls != 0 {
		t.Fatalf("composition: %v", err)
	}
	pools, err := client.Pools.List(context.Background(), 2, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pools) != 1 || pools[0].Day.APR.Fee.String() != "155.1234567890123456789" || pools[0].Day.APR.Reward.String() != "0" || pools[0].Day.APR.Total != nil || pools[0].Rewards != nil {
		t.Fatalf("source changed: %+v", pools)
	}
	if _, err := client.Pools.List(nil, 2, 100); err == nil {
		t.Fatal("nil context")
	}
	if _, err := client.Pools.List(context.Background(), 0, 100); err == nil {
		t.Fatal("bad pagination")
	}
}

// TestHTTPFailures preserves retry metadata without exposing response bodies.
//
// Version:
//   - 2026-09-19: Added.
func TestHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		status    int
		body      string
		httpError bool
	}{
		{429, "secret", true}, {503, "secret", true}, {200, "null", false}, {200, "{}", false}, {200, "broken", false},
	} {
		t.Run(tc.body+http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(tc.status)
				if _, err := io.WriteString(w, tc.body); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			c, err := NewClient(Config{BaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Pools.List(context.Background(), 1, 1)
			if err == nil {
				t.Fatal("failure accepted")
			}
			if tc.httpError {
				var h *HTTPError
				if !errors.As(err, &h) || h.RetryAfter != "120" || h.StatusCode != tc.status {
					t.Fatalf("metadata lost: %v", err)
				}
			}
		})
	}
}

// TestConstructorRejectsUnsafeURLs validates injectable endpoint configuration.
//
// Version:
//   - 2026-09-19: Added.
func TestConstructorRejectsUnsafeURLs(t *testing.T) {
	for _, u := range []string{"ftp://host", "https://user:secret@host", "https://host?token=secret", "https://host#secret"} {
		if _, err := NewClient(Config{BaseURL: u}); err == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
	c, err := NewClient(Config{})
	if err != nil || c.Pools == nil || c.Pools.baseURL != MainnetURL {
		t.Fatalf("default composition: %v", err)
	}
}
