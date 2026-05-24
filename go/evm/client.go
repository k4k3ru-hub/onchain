//
// client.go
//
package evm

import (
    "context"
    "fmt"
    "unicode/utf8"

    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/ethclient"

    "github.com/k4k3ru-hub/onchain/go/evm/erc20"
)

type Config struct {
    HTTPURL string
    WSURL   *string
}

type Client struct {
    config        Config
    httpETHClient *ethclient.Client
    wsETHClient   *ethclient.Client
}


//
// Create new EVM client.
//
// Version:
//   - 2026-05-21: Added.
//
func NewClient(ctx context.Context, config Config) (*Client, error) {
    // Guard.
    if ctx == nil {
        ctx = context.Background()
    }

    // Validate HTTP URL and dial eth client.
    httpURL := config.HTTPURL
    if httpURL == "" {
        return nil, fmt.Errorf("failed to create evm client: missing required parameter: http_url=%q", "empty")
    }
    if utf8.RuneCountInString(httpURL) > 2048 {
        return nil, fmt.Errorf("failed to create evm client: invalid parameter: http_url=%q", "too long")
    }

    httpETHClient, err := ethclient.DialContext(ctx, httpURL)
    if err != nil {
        return nil, fmt.Errorf("failed to create evm client: failed to dial evm http rpc: http_url=%q: %w", httpURL, err)
    }

    // Validate websocket URL and dial eth client. (optional)
    wsURL := config.WSURL
    var wsETHClient *ethclient.Client
    if wsURL != nil {
        if utf8.RuneCountInString(*wsURL) > 2048 {
            return nil, fmt.Errorf("failed to create evm client: invalid parameter: ws_url=%q", "too long")
        }
 
        c, err := ethclient.DialContext(ctx, *wsURL)
        if err != nil {
            return nil, fmt.Errorf("failed to create evm client: failed to dial evm ws rpc: ws_url=%q: %w", *wsURL, err)
        }
        wsETHClient = c
    }

    return &Client{
        config: config,
        httpETHClient: httpETHClient,
        wsETHClient: wsETHClient,
    }, nil
}


func (c *Client) HTTPETHClient() *ethclient.Client {
    if c == nil {
        return nil
    }
    return c.httpETHClient
}

func (c *Client) WSETHClient() *ethclient.Client {
    if c == nil {
        return nil
    }
    return c.wsETHClient
}


//
// Create new ERC20 client.
//
// Version:
//   - 2026-05-21: Added.
//
func (c *Client) ERC20(tokens []common.Address) (*erc20.Client, error) {
    // Guard.
    if c == nil {
        return nil, fmt.Errorf("unexpected nil receiver: evm client")
    }
    if c.httpETHClient == nil {
        return nil, fmt.Errorf("missing required dependency: http_eth_client")
    }

    return erc20.NewClient(c, tokens)
}
