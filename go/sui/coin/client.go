package coin

import (
	"context"
	"fmt"
	"strings"

	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type standardProvider interface {
	CoinMetadata(context.Context, string) (*onchainSui.CoinMetadata, error)
	TransactionEffects(context.Context, onchainSui.TransactionDigest) (*onchainSui.TransactionEffects, error)
	TransactionDigests(context.Context, onchainSui.TransactionQuery) (onchainSui.TransactionDigestPage, error)
}

type Client struct {
	provider           standardProvider
	transferSubscriber TransferSubscriber
	coinTypes          []string
}

// WithTransferSubscriber configures live Sui coin transfer subscriptions.
func (c *Client) WithTransferSubscriber(subscriber TransferSubscriber) *Client {
	if c != nil {
		c.transferSubscriber = subscriber
	}
	return c
}

// SubscribeTransfers subscribes to configured Sui coin transfers.
func (c *Client) SubscribeTransfers(ctx context.Context, filter TransferSubscriptionFilter) (*TransferSubscription, error) {
	if c == nil || c.transferSubscriber == nil {
		return nil, fmt.Errorf("failed to subscribe sui coin transfers: transfer_subscriber=null")
	}
	return c.transferSubscriber.SubscribeTransfers(ctx, filter)
}

// NewClient creates a Sui Coin client backed by the standard GraphQL client.
//
// Version:
//   - 2026-08-23: Added.
func NewClient(provider standardProvider, coinTypes []string) (*Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("failed to create sui coin client: provider=null")
	}
	if len(coinTypes) == 0 {
		return nil, fmt.Errorf("failed to create sui coin client: coin_types=empty")
	}
	values := make([]string, 0, len(coinTypes))
	seen := make(map[string]struct{}, len(coinTypes))
	for _, coinType := range coinTypes {
		value, err := onchainSui.NormalizeMoveType(coinType)
		if err != nil {
			return nil, fmt.Errorf("failed to create sui coin client: coin_type=invalid")
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return &Client{provider: provider, coinTypes: values}, nil
}

// CoinTypes returns the configured Sui coin types.
func (c *Client) CoinTypes() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.coinTypes...)
}

// Metadata returns metadata for a configured Sui coin type.
func (c *Client) Metadata(ctx context.Context, coinType string) (*onchainSui.CoinMetadata, error) {
	if c == nil || c.provider == nil {
		return nil, fmt.Errorf("failed to get sui coin metadata: client=null")
	}
	normalized, err := onchainSui.NormalizeMoveType(coinType)
	if err != nil || !c.hasCoinType(normalized) {
		return nil, fmt.Errorf("failed to get sui coin metadata: coin_type=not_configured")
	}
	metadata, err := c.provider.CoinMetadata(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get sui coin metadata: %w", err)
	}
	return metadata, nil
}

func (c *Client) hasCoinType(coinType string) bool {
	for _, configured := range c.coinTypes {
		if configured == strings.TrimSpace(coinType) {
			return true
		}
	}
	return false
}
