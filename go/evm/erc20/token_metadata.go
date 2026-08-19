package erc20

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	symbolMethodSelector   = crypto.Keccak256([]byte("symbol()"))[:4]
	decimalsMethodSelector = crypto.Keccak256([]byte("decimals()"))[:4]
)

type TokenMetadata struct {
	Symbol   string
	Decimals uint8
}

// GetTokenSymbol gets the ERC20 token symbol at a block.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - token: configured ERC20 token contract address.
//   - blockNumber: block number; nil uses the latest block.
//
// Returns:
//   - Token symbol returned by the contract.
//   - Symbol retrieval error.
//
// Version:
//   - 2026-08-19: Added.
func (c *Client) GetTokenSymbol(ctx context.Context, token common.Address, blockNumber *big.Int) (string, error) {
	result, err := c.callTokenMetadata(ctx, token, blockNumber, symbolMethodSelector, "symbol")
	if err != nil {
		return "", fmt.Errorf("failed to get erc20 token symbol: %w", err)
	}

	stringType, err := abi.NewType("string", "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get erc20 token symbol: failed to create string abi type: %w", err)
	}

	values, err := (abi.Arguments{{Type: stringType}}).Unpack(result)
	if err != nil {
		return "", fmt.Errorf("failed to get erc20 token symbol: failed to decode contract response: %w", err)
	}
	if len(values) != 1 {
		return "", fmt.Errorf("failed to get erc20 token symbol: decoded_values=invalid")
	}

	symbol, ok := values[0].(string)
	if !ok {
		return "", fmt.Errorf("failed to get erc20 token symbol: decoded_symbol=invalid")
	}
	if symbol == "" {
		return "", fmt.Errorf("failed to get erc20 token symbol: symbol=empty")
	}

	return symbol, nil
}

// GetTokenDecimals gets the ERC20 token decimals at a block.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - token: configured ERC20 token contract address.
//   - blockNumber: block number; nil uses the latest block.
//
// Returns:
//   - Token decimals returned by the contract.
//   - Decimals retrieval error.
//
// Version:
//   - 2026-08-19: Added.
func (c *Client) GetTokenDecimals(ctx context.Context, token common.Address, blockNumber *big.Int) (uint8, error) {
	result, err := c.callTokenMetadata(ctx, token, blockNumber, decimalsMethodSelector, "decimals")
	if err != nil {
		return 0, fmt.Errorf("failed to get erc20 token decimals: %w", err)
	}

	uint8Type, err := abi.NewType("uint8", "", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get erc20 token decimals: failed to create uint8 abi type: %w", err)
	}

	values, err := (abi.Arguments{{Type: uint8Type}}).Unpack(result)
	if err != nil {
		return 0, fmt.Errorf("failed to get erc20 token decimals: failed to decode contract response: %w", err)
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("failed to get erc20 token decimals: decoded_values=invalid")
	}

	decimals, ok := values[0].(uint8)
	if !ok {
		return 0, fmt.Errorf("failed to get erc20 token decimals: decoded_decimals=invalid")
	}

	return decimals, nil
}

// GetTokenMetadata gets ERC20 token metadata at a block.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - token: configured ERC20 token contract address.
//   - blockNumber: block number; nil uses the latest block.
//
// Returns:
//   - Token metadata returned by the contract.
//   - Metadata retrieval error.
//
// Version:
//   - 2026-08-19: Added.
func (c *Client) GetTokenMetadata(ctx context.Context, token common.Address, blockNumber *big.Int) (*TokenMetadata, error) {
	symbol, err := c.GetTokenSymbol(ctx, token, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get erc20 token metadata: %w", err)
	}

	decimals, err := c.GetTokenDecimals(ctx, token, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get erc20 token metadata: %w", err)
	}

	return &TokenMetadata{
		Symbol:   symbol,
		Decimals: decimals,
	}, nil
}

func (c *Client) callTokenMetadata(ctx context.Context, token common.Address, blockNumber *big.Int, selector []byte, method string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to call erc20 token metadata: client=null")
	}
	if c.httpClient == nil {
		return nil, fmt.Errorf("failed to call erc20 token metadata: http_client=null")
	}
	if token == (common.Address{}) {
		return nil, fmt.Errorf("failed to call erc20 token metadata: token=empty")
	}
	if blockNumber != nil && blockNumber.Sign() < 0 {
		return nil, fmt.Errorf("failed to call erc20 token metadata: block_number=out_of_range min_value=0")
	}
	if !c.hasToken(token) {
		return nil, fmt.Errorf("failed to call erc20 token metadata: token is not configured: token=%q", token.Hex())
	}
	if ctx == nil {
		ctx = context.Background()
	}

	result, err := c.httpClient.CallContract(ctx, ethereum.CallMsg{
		To:   &token,
		Data: append([]byte(nil), selector...),
	}, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to call erc20 token metadata: %w: method=%q token=%q", err, method, token.Hex())
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("failed to call erc20 token metadata: response=empty: method=%q token=%q", method, token.Hex())
	}

	return result, nil
}

func (c *Client) hasToken(token common.Address) bool {
	for _, configuredToken := range c.tokens {
		if configuredToken == token {
			return true
		}
	}
	return false
}
