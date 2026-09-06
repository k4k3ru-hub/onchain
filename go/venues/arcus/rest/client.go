// Package rest provides a composition root for Arcus public market-data APIs.
package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/k4k3ru-hub/onchain/go/venues/arcus/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/arcus/rest/markets"
)

const RobinhoodMainnetURL = "https://api.arcus.xyz"

const RobinhoodTestnetURL = "https://api.testnet.arcus.xyz"

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
type ClientParams struct {
	BaseURL          string
	HTTPClient       HTTPClient
	Timeout          time.Duration
	MaxResponseBytes int64
}
type Client struct{ Markets *markets.Client }
type httpExecutor struct {
	baseURL  url.URL
	http     HTTPClient
	timeout  time.Duration
	maxBytes int64
}
type ResponseError struct {
	StatusCode int
}

// Error describes a failed response without exposing the remote message or body.
//
// Version:
//   - 2026-09-06: Added.
func (e *ResponseError) Error() string {
	if e == nil {
		return "failed to request arcus data: response_error=null"
	}
	return fmt.Sprintf("failed to request arcus data: status_code=%d", e.StatusCode)
}

// NewClient composes all market operations with one injectable HTTP dependency.
// The default timeout is 10 seconds and the response limit is 8 MiB.
//
// Version:
//   - 2026-09-06: Added.
func NewClient(params ClientParams) (*Client, error) {
	if params.BaseURL == "" {
		params.BaseURL = RobinhoodMainnetURL
	}
	endpoint, err := safety.Endpoint(params.BaseURL, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create arcus rest client: %w", err)
	}
	if params.Timeout < 0 || params.MaxResponseBytes < 0 || params.MaxResponseBytes > 1<<30 {
		return nil, fmt.Errorf("failed to create arcus rest client: limits=out_of_range")
	}
	if params.Timeout == 0 {
		params.Timeout = 10 * time.Second
	}
	if params.MaxResponseBytes == 0 {
		params.MaxResponseBytes = 8 << 20
	}
	if params.HTTPClient == nil {
		params.HTTPClient = &http.Client{Timeout: params.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	if safety.IsNil(params.HTTPClient) {
		return nil, fmt.Errorf("failed to create arcus rest client: http_client=null")
	}
	executor := &httpExecutor{baseURL: *endpoint, http: params.HTTPClient, timeout: params.Timeout, maxBytes: params.MaxResponseBytes}
	group, err := markets.NewClient(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to create arcus rest client: %w", err)
	}
	return &Client{Markets: group}, nil
}

// Get executes a bounded public GET and decodes an Arcus response.
//
// Version:
//   - 2026-09-06: Added.
func (e *httpExecutor) Get(ctx context.Context, path string, query url.Values, result any) (err error) {
	if ctx == nil {
		return fmt.Errorf("failed to request arcus data: context=null")
	}
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	target := e.baseURL
	target.Path += path
	target.RawPath = ""
	target.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create arcus request: %w", safety.Redact(err))
	}
	req.Header.Set("Accept", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to request arcus data: %w", safety.Redact(err))
	}
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("failed to read arcus response: response_body=null")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close arcus response: %w", safety.Redact(closeErr)))
		}
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, e.maxBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read arcus response: %w", safety.Redact(err))
	}
	if int64(len(body)) > e.maxBytes {
		return fmt.Errorf("failed to read arcus response: body=too_long max_length=%d", e.maxBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ResponseError{StatusCode: resp.StatusCode}
	}
	if string(bytes.TrimSpace(body)) == "null" {
		return fmt.Errorf("failed to decode arcus response: body=null")
	}
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to decode arcus result: %w", safety.Redact(err))
	}
	return nil
}
