// client.go
package evm

import (
	"context"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/ethclient"
)

type HTTPConfig struct {
	URL string
}

type WSConfig struct {
	URL string
}

type HTTPClient struct {
	config              HTTPConfig
	ethClient           *ethclient.Client
	blockHeaderByHasher blockHeaderByHasher
	blockNumberer       blockNumberer
	contractCaller      contractCaller
	logFilterer         logFilterer
	receiptProvider     transactionReceiptProvider
	chainIDProvider     chainIDProvider
	clientCloser        clientCloser
	closeOnce           sync.Once
}

type WSClient struct {
	config        WSConfig
	ethClient     *ethclient.Client
	logSubscriber logSubscriber
}

// NewHTTPClient creates an EVM HTTP RPC client.
//
// Parameters:
//   - ctx: dial context; nil uses context.Background.
//   - config: HTTP RPC configuration.
//
// Returns:
//   - EVM HTTP RPC client.
//   - Client creation error.
//
// Version:
//   - 2026-08-17: Added.
func NewHTTPClient(ctx context.Context, config HTTPConfig) (*HTTPClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateRPCURL(config.URL, "http_url"); err != nil {
		return nil, fmt.Errorf("failed to create evm http client: %w", err)
	}

	ethClient, err := ethclient.DialContext(ctx, config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to create evm http client: failed to dial evm http rpc: %w", err)
	}

	return composeHTTPClient(config, ethClient), nil
}

// NewWSClient creates an EVM WebSocket RPC client.
//
// Parameters:
//   - ctx: dial context; nil uses context.Background.
//   - config: WebSocket RPC configuration.
//
// Returns:
//   - EVM WebSocket RPC client.
//   - Client creation error.
//
// Version:
//   - 2026-08-17: Added.
func NewWSClient(ctx context.Context, config WSConfig) (*WSClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateRPCURL(config.URL, "ws_url"); err != nil {
		return nil, fmt.Errorf("failed to create evm ws client: %w", err)
	}

	ethClient, err := ethclient.DialContext(ctx, config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to create evm ws client: failed to dial evm ws rpc: %w", err)
	}

	return composeWSClient(config, ethClient), nil
}

func composeHTTPClient(config HTTPConfig, ethClient *ethclient.Client) *HTTPClient {
	client := &HTTPClient{
		config:    config,
		ethClient: ethClient,
	}
	if ethClient != nil {
		client.blockHeaderByHasher = ethClient
		client.blockNumberer = ethClient
		client.contractCaller = ethClient
		client.logFilterer = ethClient
		client.receiptProvider = ethClient
		client.chainIDProvider = ethClient
		client.clientCloser = ethClient
	}

	return client
}

func composeWSClient(config WSConfig, ethClient *ethclient.Client) *WSClient {
	client := &WSClient{
		config:    config,
		ethClient: ethClient,
	}
	if ethClient != nil {
		client.logSubscriber = ethClient
	}

	return client
}

func validateRPCURL(url, parameterName string) error {
	if url == "" {
		return fmt.Errorf("failed to validate evm rpc url: %s=empty", parameterName)
	}
	if utf8.RuneCountInString(url) > 2048 {
		return fmt.Errorf("failed to validate evm rpc url: %s=too_long actual_length=%d max_length=2048", parameterName, utf8.RuneCountInString(url))
	}

	return nil
}
