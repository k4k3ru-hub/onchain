//
// client.go
//
package erc20

import (
    "fmt"

    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/ethclient"
)


type evmClient interface {
    HTTPETHClient() *ethclient.Client
    WSETHClient() *ethclient.Client
}

type Client struct {
    evm    evmClient
    tokens []common.Address
}


//
// Create new ERC20 client.
//
// Version:
//   - 2026-05-21: Added.
//
func NewClient(evm evmClient, tokens []common.Address) (*Client, error) {
    // Guard.
    if evm == nil {
        return nil, fmt.Errorf("failed to create erc20 client: missing required parameter: evm=null")
    }

    // Validate HTTP ETH client.
    if evm.HTTPETHClient() == nil {
        return nil, fmt.Errorf("failed to create erc20 client: missing required dependency: http_eth_client=null")
    }

    // Validate tokens.
    copiedTokens := make([]common.Address, 0, len(tokens))
    for _, token := range tokens {
        if token == (common.Address{}) {
            continue
        }
        copiedTokens = append(copiedTokens, token)
    }
    if len(copiedTokens) == 0 {
        return nil, fmt.Errorf("failed to create erc20 client: missing required parameter: tokens=%q", "empty")
    }

    return &Client{
        evm:    evm,
        tokens: copiedTokens,
    }, nil
}

func (c *Client) HTTPETHClient() *ethclient.Client {
    if c == nil || c.evm == nil {
        return nil
    }
    return c.evm.HTTPETHClient()
}

func (c *Client) WSETHClient() *ethclient.Client {
    if c == nil || c.evm == nil {
        return nil
    }
    return c.evm.WSETHClient()
}

func (c *Client) Tokens() []common.Address {
    if c == nil || len(c.tokens) == 0 {
        return nil
    }

    tokens := make([]common.Address, len(c.tokens))
    copy(tokens, c.tokens)

    return tokens
}
