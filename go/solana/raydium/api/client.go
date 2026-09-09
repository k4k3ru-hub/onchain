// Package api reads Raydium's indexed pool statistics independently of CLMM quotes.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/k4k3ru-hub/onchain/go/solana"
)

const MainnetURL = "https://api-v3.raydium.io"
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
//   - 2026-09-09: Added.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("failed to read raydium api response: status_code=%d", e.StatusCode)
}

type transportError struct{ cause error }

// Error hides transport details that may include credentials.
//
// Version:
//   - 2026-09-09: Added.
func (e *transportError) Error() string {
	return "failed to send raydium api request: transport failure"
}

// Unwrap preserves the underlying transport error for inspection.
//
// Version:
//   - 2026-09-09: Added.
func (e *transportError) Unwrap() error { return e.cause }

// NewClient composes the pool API group with an injectable HTTP client.
// No goroutines, polling, retries or network requests are started by construction.
// An omitted URL selects mainnet; an omitted HTTP client uses a 15-second timeout.
//
// Version:
//   - 2026-09-09: Added.
func NewClient(config Config) (*Client, error) {
	base := config.BaseURL
	if base == "" {
		base = MainnetURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("failed to create raydium api client: base_url=invalid")
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("failed to create raydium api client: base_url=invalid")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{Pools: &PoolsClient{baseURL: strings.TrimRight(base, "/"), http: client}}, nil
}

// Get reads indexed statistics for exactly one pool, without quoting or retrying.
// Missing statistics remain nil and provider APR units are preserved.
//
// Version:
//   - 2026-09-09: Added.
func (c *PoolsClient) Get(ctx context.Context, pool solana.Address) (*Pool, error) {
	if pool == (solana.Address{}) {
		return nil, fmt.Errorf("failed to get raydium pool: pool=empty")
	}
	var response struct {
		Success *bool   `json:"success"`
		Data    []*Pool `json:"data"`
	}
	if err := c.get(ctx, "/pools/info/ids?ids="+url.QueryEscape(pool.String()), &response); err != nil {
		return nil, fmt.Errorf("failed to get raydium pool: %w", err)
	}
	if response.Success != nil && !*response.Success {
		return nil, fmt.Errorf("failed to get raydium pool: upstream response unsuccessful")
	}
	if len(response.Data) != 1 || response.Data[0] == nil {
		return nil, fmt.Errorf("failed to get raydium pool: data=invalid")
	}
	result := response.Data[0]
	id, err := solana.ParseAddress(result.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get raydium pool: %w", err)
	}
	if id != pool {
		return nil, fmt.Errorf("failed to get raydium pool: pool_id=invalid")
	}
	return result, nil
}

func (c *PoolsClient) get(ctx context.Context, path string, out any) (err error) {
	if c == nil || c.http == nil {
		return fmt.Errorf("failed to get raydium api data: client=null")
	}
	if ctx == nil {
		return fmt.Errorf("failed to get raydium api data: context=null")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return &transportError{cause: err}
	}
	req.Header.Set("Accept", "application/json")
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return &transportError{cause: err}
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("failed to read raydium api data: response=too_long")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("failed to decode raydium api data: %w", err)
	}
	return nil
}
