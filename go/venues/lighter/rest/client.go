// Package rest provides a composition root for Lighter public market-data APIs.
package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/k4k3ru-hub/onchain/go/venues/lighter/internal/safety"
	"github.com/k4k3ru-hub/onchain/go/venues/lighter/rest/markets"
)

const RobinhoodMainnetURL = "https://api.rh.lighter.xyz"

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
	Code       int
}

// Error describes a failed response without exposing the remote message or body.
//
// Version:
//   - 2026-09-06: Added.
func (e *ResponseError) Error() string {
	if e == nil {
		return "failed to request lighter data: response_error=null"
	}
	return fmt.Sprintf("failed to request lighter data: status_code=%d code=%d", e.StatusCode, e.Code)
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
		return nil, fmt.Errorf("failed to create lighter rest client: %w", err)
	}
	if params.Timeout < 0 || params.MaxResponseBytes < 0 || params.MaxResponseBytes > 1<<30 {
		return nil, fmt.Errorf("failed to create lighter rest client: limits=out_of_range")
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
		return nil, fmt.Errorf("failed to create lighter rest client: http_client=null")
	}
	executor := &httpExecutor{baseURL: *endpoint, http: params.HTTPClient, timeout: params.Timeout, maxBytes: params.MaxResponseBytes}
	group, err := markets.NewClient(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to create lighter rest client: %w", err)
	}
	return &Client{Markets: group}, nil
}

// Get executes a bounded public GET and decodes a successful Lighter envelope.
//
// Version:
//   - 2026-09-06: Added.
func (e *httpExecutor) Get(ctx context.Context, path string, query url.Values, result any) (err error) {
	if ctx == nil {
		return fmt.Errorf("failed to request lighter data: context=null")
	}
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	target := e.baseURL
	target.Path += path
	target.RawPath = ""
	target.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create lighter request: %w", safety.Redact(err))
	}
	req.Header.Set("Accept", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to request lighter data: %w", safety.Redact(err))
	}
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("failed to read lighter response: response_body=null")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close lighter response: %w", safety.Redact(closeErr)))
		}
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, e.maxBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read lighter response: %w", safety.Redact(err))
	}
	if int64(len(body)) > e.maxBytes {
		return fmt.Errorf("failed to read lighter response: body=too_long max_length=%d", e.maxBytes)
	}
	var envelope struct {
		Code *int `json:"code"`
	}
	decodeErr := json.Unmarshal(body, &envelope)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := 0
		if envelope.Code != nil {
			code = *envelope.Code
		}
		return &ResponseError{StatusCode: resp.StatusCode, Code: code}
	}
	if decodeErr != nil {
		return fmt.Errorf("failed to decode lighter response: %w", safety.Redact(decodeErr))
	}
	if envelope.Code == nil {
		return fmt.Errorf("failed to decode lighter response: code=null")
	}
	if *envelope.Code != 200 {
		return &ResponseError{StatusCode: resp.StatusCode, Code: *envelope.Code}
	}
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to decode lighter result: %w", safety.Redact(err))
	}
	return nil
}
