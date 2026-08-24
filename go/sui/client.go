package sui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type RPCConfig struct{ URL string }

type RPCClient struct {
	config RPCConfig
	caller graphQLCaller
}

type graphQLCaller interface {
	query(context.Context, string, any) error
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type graphQLAdapter struct {
	url    string
	client httpDoer
}

// NewRPCClient creates a Sui GraphQL RPC client.
//
// Parameters:
//   - ctx: construction context; nil uses context.Background.
//   - config: Sui RPC configuration.
//
// Returns:
//   - Sui RPC client.
//   - Client creation error.
//
// Version:
//   - 2026-08-22: Added.
func NewRPCClient(ctx context.Context, config RPCConfig) (*RPCClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create sui rpc client: %w", err)
	}
	adapter := &graphQLAdapter{url: strings.TrimSpace(config.URL), client: http.DefaultClient}
	return composeRPCClient(config, adapter), nil
}

// Validate validates the Sui RPC configuration.
func (c RPCConfig) Validate() error {
	value := strings.TrimSpace(c.URL)
	if value == "" {
		return fmt.Errorf("failed to validate sui rpc config: rpc_url=empty")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("failed to validate sui rpc config: rpc_url=invalid")
	}
	return nil
}

func composeRPCClient(config RPCConfig, caller graphQLCaller) *RPCClient {
	return &RPCClient{config: config, caller: caller}
}

func (a *graphQLAdapter) query(ctx context.Context, query string, result any) error {
	body, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return fmt.Errorf("failed to query sui graphql: failed to encode request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to query sui graphql: failed to create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("failed to query sui graphql: failed to send request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, response.Body)
		return fmt.Errorf("failed to query sui graphql: http_status=%d", response.StatusCode)
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("failed to query sui graphql: failed to decode response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("failed to query sui graphql: graphql_message=%q", envelope.Errors[0].Message)
	}
	if err := json.Unmarshal(envelope.Data, result); err != nil {
		return fmt.Errorf("failed to query sui graphql: failed to decode data: %w", err)
	}
	return nil
}
