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

	"github.com/k4k3ru-hub/onchain/go/sui"
)

func poolAddress(t *testing.T, value string) sui.Address {
	t.Helper()
	p, err := sui.ParseAddress(value)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestGetCapturedStatistics verifies the v3 request and source-labelled APRs.
//
// Version:
//   - 2026-09-14: Added.
func TestGetCapturedStatistics(t *testing.T) {
	body, err := os.ReadFile("testdata/stats_pools_v3.json")
	if err != nil {
		t.Fatal(err)
	}
	pool := poolAddress(t, "0x51e883ba7c0b566a26cbc8a94cd33eb0abd418a77cc1e60ad22fd9b1f29cd2ab")
	calls := 0
	c := fakeClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v3/sui/clmm/stats_pools" || r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
			t.Fatalf("unexpected request: %+v", r)
		}
		var request struct {
			Pools           []string `json:"pools"`
			DisplayAllPools bool     `json:"display_all_pools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Pools) != 1 || request.Pools[0] != pool.String() || !request.DisplayAllPools {
			t.Fatalf("unexpected selector: %+v", request)
		}
		return response(string(body)), nil
	})
	if calls != 0 || c.Pools == nil {
		t.Fatal("invalid composition")
	}
	got, err := c.Pools.Get(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got.Pool != pool.String() || got.TotalAPR.String() != "0.4080618520740995" || got.TVL.String() != "647562.5322253264916193574016187" {
		t.Fatalf("lost precision or identity: %+v", got)
	}
	if len(got.Stats) != 3 || got.Stats[0].DateType != "24H" || got.Stats[0].APR.String() != "0.225501667331869" || got.Stats[1].DateType != "7D" || got.Stats[2].DateType != "30D" {
		t.Fatal("lost source periods")
	}
	if len(got.MiningRewarders) != 1 || got.MiningRewarders[0].CoinType != "0x06864a6f921804860930db6ddbe2e16acdf8504495ea7481637a1c8b9a8fe54b::cetus::CETUS" || got.MiningRewarders[0].APR.String() != "0.1825601847422305" || got.MiningRewarders[0].EmissionsPerSecond.String() != "0.1736111111111111" {
		t.Fatal("lost reward attribution")
	}
	if got.CoinA == nil || *got.CoinA.Decimals != 6 || got.Vault == nil || got.Extensions == nil || got.Raw == nil {
		t.Fatal("lost metadata")
	}
}

// TestGetValidationAndFailures rejects unrelated pools and preserves inspectable failures.
//
// Version:
//   - 2026-09-14: Added.
func TestGetValidationAndFailures(t *testing.T) {
	pool := poolAddress(t, "0x1")
	for _, body := range []string{
		`null`, `{}`, `{"code":0,"data":null}`, `{"code":0,"data":{"list":[]}}`,
		`{"code":0,"data":{"total":0,"list":null}}`,
		`{"code":0,"data":{"total":1,"list":[]}}`,
		`{"code":0,"data":{"total":1,"list":[null]}}`,
		`{"code":0,"data":{"total":1,"list":[{"pool":"0x2"}]}}`,
		`{"code":0,"data":{"total":1,"list":[{"pool":"invalid"}]}}`,
		`{"code":0,"data":{"total":2,"list":[{"pool":"0x1"},{"pool":"0x1"}]}}`,
		`{"code":0,"data":{"total":1,"list":[{"pool":"0x1","totalApr":"NaN"}]}}`,
		`{"code":0,"data":{"total":1,"list":[{"pool":"0x1","stats":[{"apr":"bad"}]}]}}`,
	} {
		t.Run(body, func(t *testing.T) {
			c := fakeClient(t, func(*http.Request) (*http.Response, error) { return response(body), nil })
			if got, err := c.Pools.Get(context.Background(), pool); err == nil || got != nil || errors.Is(err, ErrPoolNotFound) {
				t.Fatalf("invalid response: %v %v", got, err)
			}
		})
	}
	c := fakeClient(t, func(*http.Request) (*http.Response, error) {
		return response(`{"code":0,"data":{"total":0,"list":[]}}`), nil
	})
	if _, err := c.Pools.Get(context.Background(), pool); !errors.Is(err, ErrPoolNotFound) {
		t.Fatal(err)
	}
	c = fakeClient(t, func(*http.Request) (*http.Response, error) {
		return response(`{"code":123,"msg":"confidential","data":null}`), nil
	})
	_, err := c.Pools.Get(context.Background(), pool)
	var upstream *APIError
	if !errors.As(err, &upstream) || upstream.Code != 123 || strings.Contains(err.Error(), "confidential") {
		t.Fatal(err)
	}
	calls := 0
	c = fakeClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		r := response("confidential")
		r.StatusCode = 429
		r.Header.Set("Retry-After", "30")
		return r, nil
	})
	_, err = c.Pools.Get(context.Background(), pool)
	var status *HTTPError
	if !errors.As(err, &status) || status.RetryAfter != "30" || calls != 1 || strings.Contains(err.Error(), "confidential") {
		t.Fatal(err)
	}
	c = fakeClient(t, func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Pools.Get(ctx, pool); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c = fakeClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network request"); return nil, io.EOF })
	if _, err := c.Pools.Get(context.Background(), sui.Address{}); err == nil {
		t.Fatal("zero pool accepted")
	}
	if _, err := c.Pools.Get(nil, pool); err == nil {
		t.Fatal("nil context accepted")
	}
	var empty *PoolsClient
	if _, err := empty.Get(context.Background(), pool); err == nil {
		t.Fatal("nil client accepted")
	}
}

// TestGetMissingMetrics retains missing 24h metrics and unknown provider metadata.
//
// Version:
//   - 2026-09-14: Added.
func TestGetMissingMetrics(t *testing.T) {
	c := fakeClient(t, func(*http.Request) (*http.Response, error) {
		return response(`{"code":0,"data":{"total":1,"list":[{"pool":"0x1","totalApr":null,"stats":[{"dateType":"7D","apr":"0"}],"miningRewarders":[{"coinType":"reward","apr":null}],"futureFarm":{"amount":9007199254740993}}]}}`), nil
	})
	got, err := c.Pools.Get(context.Background(), poolAddress(t, "0x1"))
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalAPR != nil || got.TVL != nil || len(got.Stats) != 1 || got.Stats[0].DateType != "7D" || got.Stats[0].APR.String() != "0" || got.MiningRewarders[0].APR != nil || !strings.Contains(string(got.Raw), `"amount":9007199254740993`) {
		t.Fatal("metrics synthesized or lost")
	}
}
