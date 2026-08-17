// client.go
package erc20

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type httpClient interface {
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

type wsClient interface {
	SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error)
}

type Client struct {
	httpClient httpClient
	wsClient   wsClient
	tokens     []common.Address
}

// NewClient creates an ERC20 client.
//
// Parameters:
//   - httpClient: EVM HTTP operations.
//   - wsClient: optional EVM WebSocket operations.
//   - tokens: ERC20 token contract addresses.
//
// Returns:
//   - ERC20 client.
//   - Client creation error.
//
// Version:
//   - 2026-08-17: Changed to accept separate HTTP and WebSocket dependencies.
//   - 2026-05-21: Added.
func NewClient(httpClient httpClient, wsClient wsClient, tokens []common.Address) (*Client, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("failed to create erc20 client: http_client=null")
	}

	copiedTokens := make([]common.Address, 0, len(tokens))
	for _, token := range tokens {
		if token == (common.Address{}) {
			continue
		}
		copiedTokens = append(copiedTokens, token)
	}
	if len(copiedTokens) == 0 {
		return nil, fmt.Errorf("failed to create erc20 client: tokens=empty")
	}

	return &Client{
		httpClient: httpClient,
		wsClient:   wsClient,
		tokens:     copiedTokens,
	}, nil
}

// Tokens returns the configured ERC20 token contract addresses.
//
// Returns:
//   - Copy of the configured token addresses.
//
// Version:
//   - 2026-08-17: Added GoDoc.
//   - 2026-05-21: Added.
func (c *Client) Tokens() []common.Address {
	if c == nil || len(c.tokens) == 0 {
		return nil
	}

	tokens := make([]common.Address, len(c.tokens))
	copy(tokens, c.tokens)

	return tokens
}
