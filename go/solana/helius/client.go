package helius

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
	"github.com/k4k3ru-hub/onchain/go/solana/spltoken"
)

type RPCConfig struct {
	BaseURL    string
	APIKey     string
	Commitment onchainSolana.Commitment
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}
type transactionSource interface {
	Transaction(context.Context, onchainSolana.Signature) (*onchainSolana.Transaction, error)
}

type RPCClient struct {
	config       RPCConfig
	client       httpDoer
	transactions transactionSource
}

// NewRPCClient creates a Helius transaction-history client.
func NewRPCClient(config RPCConfig) (*RPCClient, error) {
	return NewRPCClientWithContext(context.Background(), config)
}

// NewRPCClientWithContext creates a Helius transaction-history client with a construction context.
//
// Parameters:
//   - ctx: construction context; nil uses context.Background.
//   - config: Helius RPC configuration.
//
// Returns:
//   - Helius RPC client.
//   - Client creation error.
//
// Version:
//   - 2026-08-22: Added.
func NewRPCClientWithContext(ctx context.Context, config RPCConfig) (*RPCClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create helius rpc client: %w", err)
	}
	endpoint, err := endpointURL(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create helius rpc client: %w", err)
	}
	transactions, err := onchainSolana.NewRPCClient(ctx, onchainSolana.RPCConfig{URL: endpoint, Commitment: config.Commitment})
	if err != nil {
		return nil, fmt.Errorf("failed to create helius rpc client: %w", err)
	}
	return &RPCClient{config: config, client: http.DefaultClient, transactions: transactions}, nil
}

// Validate validates Helius RPC configuration.
func (c RPCConfig) Validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("failed to validate helius rpc config: base_url=empty")
	}
	parsed, err := url.ParseRequestURI(strings.TrimSpace(c.BaseURL))
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
		return fmt.Errorf("failed to validate helius rpc config: base_url=invalid")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("failed to validate helius rpc config: api_key=empty")
	}
	if c.Commitment != onchainSolana.CommitmentConfirmed && c.Commitment != onchainSolana.CommitmentFinalized {
		return fmt.Errorf("failed to validate helius rpc config: commitment=invalid")
	}
	return nil
}

// TransferEvents returns SPL Token transfers using Helius getTransactionsForAddress.
func (c *RPCClient) TransferEvents(ctx context.Context, filter spltoken.TransferFilter) ([]spltoken.TransferEvent, error) {
	page, err := c.TransferEventPage(ctx, filter)
	if err != nil {
		return nil, err
	}
	return page.Events, nil
}

// TransferEventPage returns Helius SPL Token transfers and the next pagination token.
func (c *RPCClient) TransferEventPage(ctx context.Context, filter spltoken.TransferFilter) (*spltoken.TransferPage, error) {
	if c == nil || c.client == nil || c.transactions == nil {
		return nil, fmt.Errorf("failed to get helius token transfer page: client=null")
	}
	if filter.Address.IsZero() {
		return nil, fmt.Errorf("failed to get helius token transfers: address=empty")
	}
	if filter.Limit < 1 || filter.Limit > 1000 {
		return nil, fmt.Errorf("failed to get helius token transfers: limit=out_of_range min_value=1 max_value=1000")
	}
	configuration := map[string]any{"transactionDetails": "signatures", "limit": filter.Limit, "sortOrder": "desc", "commitment": c.config.Commitment.String()}
	filters := map[string]any{"status": "succeeded", "tokenAccounts": "balanceChanged", "tokenTransfer": map[string]any{"direction": "any"}}
	if filter.Mint != nil {
		filters["tokenTransfer"].(map[string]any)["mint"] = filter.Mint.String()
	}
	if filter.MinSlot != nil || filter.MaxSlot != nil {
		slots := map[string]any{}
		if filter.MinSlot != nil {
			slots["gte"] = filter.MinSlot.Uint64()
		}
		if filter.MaxSlot != nil {
			slots["lte"] = filter.MaxSlot.Uint64()
		}
		filters["slot"] = slots
	}
	configuration["filters"] = filters
	if filter.Cursor != "" {
		configuration["paginationToken"] = filter.Cursor
	}
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "1", "method": "getTransactionsForAddress", "params": []any{filter.Address.String(), configuration}})
	if err != nil {
		return nil, fmt.Errorf("failed to get helius token transfers: failed to encode request: %w", err)
	}
	endpoint, err := endpointURL(c.config)
	if err != nil {
		return nil, fmt.Errorf("failed to get helius token transfers: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to get helius token transfers: failed to create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to get helius token transfers: failed to send request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to get helius token transfers: http_status=%d", response.StatusCode)
	}
	var envelope struct {
		Result struct {
			Data []struct {
				Signature string `json:"signature"`
			} `json:"data"`
			PaginationToken string `json:"paginationToken"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("failed to get helius token transfers: failed to decode response: %w", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("failed to get helius token transfers: rpc_code=%d rpc_message=%q", envelope.Error.Code, envelope.Error.Message)
	}
	var events []spltoken.TransferEvent
	for _, value := range envelope.Result.Data {
		signature, err := onchainSolana.ParseSignature(value.Signature)
		if err != nil {
			return nil, fmt.Errorf("failed to get helius token transfers: %w", err)
		}
		transaction, err := c.transactions.Transaction(ctx, signature)
		if err != nil {
			return nil, fmt.Errorf("failed to get helius token transfers: %w", err)
		}
		for _, event := range spltoken.ParseTransactionTransfers(transaction, filter.Mint) {
			if event.From == filter.Address || event.To == filter.Address {
				events = append(events, event)
			}
		}
	}
	return &spltoken.TransferPage{Events: events, NextCursor: envelope.Result.PaginationToken}, nil
}

func endpointURL(config RPCConfig) (string, error) {
	endpoint, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil {
		return "", fmt.Errorf("failed to build helius endpoint: base_url=invalid")
	}
	query := endpoint.Query()
	query.Set("api-key", config.APIKey)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}
