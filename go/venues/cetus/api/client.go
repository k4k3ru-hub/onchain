// Package api reads Cetus's indexed pool statistics independently of CLMM quotes.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/k4k3ru-hub/onchain/go/sui"
)

const MainnetURL = "https://api-sui.cetus.zone"
const maxResponseBytes = 16 << 20

type Config struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct{ Pools *PoolsClient }
type PoolsClient struct {
	baseURL string
	http    *http.Client
}
type HTTPError struct {
	StatusCode int
	RetryAfter string
}

// Error describes an unsuccessful HTTP response without including its body.
//
// Version:
//   - 2026-09-14: Added.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("failed to read cetus api response: status_code=%d", e.StatusCode)
}

type transportError struct{ cause error }

// Error hides transport details that may include credentials.
//
// Version:
//   - 2026-09-14: Added.
func (e *transportError) Error() string {
	return "failed to send cetus api request: transport failure"
}

// Unwrap preserves the underlying transport error for inspection.
//
// Version:
//   - 2026-09-14: Added.
func (e *transportError) Unwrap() error { return e.cause }

// NewClient composes the pool API group with an injectable HTTP client.
// No goroutines, polling, retries or network requests are started by construction.
// An omitted URL selects mainnet; an omitted HTTP client uses a 15-second timeout.
//
// Version:
//   - 2026-09-14: Added.
func NewClient(config Config) (*Client, error) {
	base := config.BaseURL
	if base == "" {
		base = MainnetURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("failed to create cetus api client: %w: base_url=invalid", &detailError{operation: "failed to parse base url", cause: err})
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("failed to create cetus api client: base_url=invalid")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{Pools: &PoolsClient{baseURL: strings.TrimRight(base, "/"), http: client}}, nil
}

// List returns one indexed statistics page with source units and missing values preserved.
// Zero Limit omits the parameter and uses the provider default. Offset is zero-based.
// Total is the provider's reported count, not a guarantee of snapshot-consistent pagination.
// No automatic pagination, APR calculation, reward attribution or fallback is performed.
//
// Version:
//   - 2026-09-14: Added.
func (c *PoolsClient) List(ctx context.Context, params ListParams) (*PoolPage, error) {
	if params.Limit < 0 || params.Offset < 0 {
		return nil, fmt.Errorf("failed to list cetus pools: pagination=out_of_range")
	}
	query := url.Values{}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	}
	query.Set("offset", strconv.Itoa(params.Offset))
	var response struct {
		Code *int      `json:"code"`
		Data *PoolPage `json:"data"`
	}
	if err := c.get(ctx, "/v2/sui/stats_pools?"+query.Encode(), &response); err != nil {
		return nil, fmt.Errorf("failed to list cetus pools: %w", err)
	}
	if response.Code == nil {
		return nil, fmt.Errorf("failed to list cetus pools: code=null")
	}
	if *response.Code != 0 {
		return nil, fmt.Errorf("failed to list cetus pools: %w", &APIError{Code: *response.Code})
	}
	if response.Data == nil || response.Data.Total == nil || response.Data.Pools == nil {
		return nil, fmt.Errorf("failed to list cetus pools: data=null")
	}
	if *response.Data.Total < 0 {
		return nil, fmt.Errorf("failed to list cetus pools: total=out_of_range")
	}
	for _, p := range response.Data.Pools {
		if err := p.validate(); err != nil {
			return nil, fmt.Errorf("failed to list cetus pools: %w", err)
		}
	}
	return response.Data, nil
}

// Get reads v3 indexed statistics for exactly the requested pool.
// It preserves period labels and source units without pagination or fallback to v2.
// An empty result returns ErrPoolNotFound; mismatched or ambiguous results are errors.
//
// Version:
//   - 2026-09-14: Added.
func (c *PoolsClient) Get(ctx context.Context, pool sui.Address) (*PoolStats, error) {
	if pool.IsZero() {
		return nil, fmt.Errorf("failed to get cetus pool: pool=empty")
	}
	body, err := json.Marshal(struct {
		Pools           []string `json:"pools"`
		DisplayAllPools bool     `json:"display_all_pools"`
	}{Pools: []string{pool.String()}, DisplayAllPools: true})
	if err != nil {
		return nil, fmt.Errorf("failed to get cetus pool: %w", err)
	}
	var response struct {
		Code *int `json:"code"`
		Data *struct {
			Total *int64       `json:"total"`
			List  []*PoolStats `json:"list"`
		} `json:"data"`
	}
	if err := c.request(ctx, http.MethodPost, "/v3/sui/clmm/stats_pools", bytes.NewReader(body), &response); err != nil {
		return nil, fmt.Errorf("failed to get cetus pool: %w", err)
	}
	if response.Code == nil {
		return nil, fmt.Errorf("failed to get cetus pool: code=null")
	}
	if *response.Code != 0 {
		return nil, fmt.Errorf("failed to get cetus pool: %w", &APIError{Code: *response.Code})
	}
	if response.Data == nil || response.Data.Total == nil || response.Data.List == nil {
		return nil, fmt.Errorf("failed to get cetus pool: data=null")
	}
	data := response.Data
	if *data.Total == 0 && len(data.List) == 0 {
		return nil, fmt.Errorf("failed to get cetus pool: %w: pool_id=%q", ErrPoolNotFound, pool.String())
	}
	if *data.Total != 1 || len(data.List) != 1 || data.List[0] == nil {
		return nil, fmt.Errorf("failed to get cetus pool: data=invalid")
	}
	result := data.List[0]
	id, err := sui.ParseAddress(result.Pool)
	if err != nil {
		return nil, fmt.Errorf("failed to get cetus pool: %w", err)
	}
	if id != pool {
		return nil, fmt.Errorf("failed to get cetus pool: pool_id=invalid")
	}
	return result, nil
}

func (c *PoolsClient) get(ctx context.Context, path string, out any) error {
	return c.request(ctx, http.MethodGet, path, nil, out)
}

func (c *PoolsClient) request(ctx context.Context, method, path string, body io.Reader, out any) (err error) {
	if c == nil || c.http == nil {
		return fmt.Errorf("failed to get cetus api data: client=null")
	}
	if ctx == nil {
		return fmt.Errorf("failed to get cetus api data: context=null")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return &transportError{cause: err}
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return &transportError{cause: err}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, &transportError{cause: closeErr})
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return &transportError{cause: err}
	}
	if len(payload) > maxResponseBytes {
		return fmt.Errorf("failed to read cetus api data: response=too_long")
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return &detailError{operation: "failed to decode cetus api data: response=invalid", cause: err}
	}
	return nil
}
