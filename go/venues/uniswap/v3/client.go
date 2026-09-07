package v3

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
)

type HTTPRPCClient interface {
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

type WSRPCClient interface {
	SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error)
}

type FactoryConfig struct {
	Address      common.Address
	QuoterV2     common.Address
	InitCodeHash common.Hash
	PoolKeys     []protocol.PoolKey
}

type HTTPClientParams struct {
	RPC     HTTPRPCClient
	Factory FactoryConfig
}

type WSClientParams struct {
	RPC     WSRPCClient
	Factory FactoryConfig
}

type ClientParams struct {
	HTTPRPCClient HTTPRPCClient
	WSRPCClient   WSRPCClient
	Factory       FactoryConfig
}

type HTTPClient struct {
	rpc       HTTPRPCClient
	quoterV2  common.Address
	poolKeys  []protocol.PoolKey
	poolByAdd map[common.Address]protocol.PoolKey
}

type WSClient struct {
	rpc       WSRPCClient
	poolKeys  []protocol.PoolKey
	poolByAdd map[common.Address]protocol.PoolKey
}

type Client struct {
	HTTP *HTTPClient
	WS   *WSClient
}

// NewHTTPClient creates a Uniswap v3 HTTP client.
//
// Parameters:
//   - params: HTTP RPC dependency and factory configuration.
//
// Returns:
//   - Uniswap v3 HTTP client.
//   - Client creation error.
//
// Version:
//   - 2026-08-24: Added.
func NewHTTPClient(params HTTPClientParams) (*HTTPClient, error) {
	if params.RPC == nil {
		return nil, fmt.Errorf("failed to create uniswap v3 http client: http_rpc_client=null")
	}
	poolKeys, poolByAddress, err := validateFactory(params.Factory, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create uniswap v3 http client: %w", err)
	}
	return &HTTPClient{rpc: params.RPC, quoterV2: params.Factory.QuoterV2, poolKeys: poolKeys, poolByAdd: poolByAddress}, nil
}

// NewWSClient creates a Uniswap v3 WebSocket client.
//
// Parameters:
//   - params: WebSocket RPC dependency and factory configuration.
//
// Returns:
//   - Uniswap v3 WebSocket client.
//   - Client creation error.
//
// Version:
//   - 2026-08-24: Added.
func NewWSClient(params WSClientParams) (*WSClient, error) {
	if params.RPC == nil {
		return nil, fmt.Errorf("failed to create uniswap v3 ws client: ws_rpc_client=null")
	}
	poolKeys, poolByAddress, err := validateFactory(params.Factory, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create uniswap v3 ws client: %w", err)
	}
	return &WSClient{rpc: params.RPC, poolKeys: poolKeys, poolByAdd: poolByAddress}, nil
}

// NewClient creates a complete Uniswap v3 client.
//
// Parameters:
//   - params: HTTP and WebSocket dependencies and factory configuration.
//
// Returns:
//   - Composed Uniswap v3 client.
//   - Client creation error.
//
// Version:
//   - 2026-08-24: Added.
func NewClient(params ClientParams) (*Client, error) {
	httpClient, err := NewHTTPClient(HTTPClientParams{RPC: params.HTTPRPCClient, Factory: params.Factory})
	if err != nil {
		return nil, fmt.Errorf("failed to create uniswap v3 client: %w", err)
	}
	wsClient, err := NewWSClient(WSClientParams{RPC: params.WSRPCClient, Factory: params.Factory})
	if err != nil {
		return nil, fmt.Errorf("failed to create uniswap v3 client: %w", err)
	}
	return &Client{HTTP: httpClient, WS: wsClient}, nil
}

func validateFactory(config FactoryConfig, requireQuoter bool) ([]protocol.PoolKey, map[common.Address]protocol.PoolKey, error) {
	if config.Address == (common.Address{}) {
		return nil, nil, fmt.Errorf("failed to validate uniswap v3 factory config: factory=empty")
	}
	if requireQuoter && config.QuoterV2 == (common.Address{}) {
		return nil, nil, fmt.Errorf("failed to validate uniswap v3 factory config: quoter_v2=empty")
	}
	if config.InitCodeHash == (common.Hash{}) {
		return nil, nil, fmt.Errorf("failed to validate uniswap v3 factory config: init_code_hash=empty")
	}
	if len(config.PoolKeys) == 0 {
		return nil, nil, fmt.Errorf("failed to validate uniswap v3 factory config: pool_keys=empty")
	}
	poolKeys := make([]protocol.PoolKey, len(config.PoolKeys))
	poolByAddress := make(map[common.Address]protocol.PoolKey, len(config.PoolKeys))
	for i, poolKey := range config.PoolKeys {
		if err := poolKey.Validate(); err != nil {
			return nil, nil, fmt.Errorf("failed to validate uniswap v3 factory config: %w: pool_index=%d", err, i)
		}
		poolAddress, err := poolKey.Address(config.Address, config.InitCodeHash)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to validate uniswap v3 factory config: %w: pool_index=%d", err, i)
		}
		if _, exists := poolByAddress[poolAddress]; exists {
			return nil, nil, fmt.Errorf("failed to validate uniswap v3 factory config: duplicate pool key: pool_address=%q pool_index=%d", poolAddress.Hex(), i)
		}
		poolKeys[i] = poolKey
		poolByAddress[poolAddress] = poolKey
	}
	return poolKeys, poolByAddress, nil
}

func configuredPool(poolByAddress map[common.Address]protocol.PoolKey, poolAddress common.Address) (protocol.PoolKey, bool) {
	poolKey, ok := poolByAddress[poolAddress]
	return poolKey, ok
}

func poolAddresses(poolByAddress map[common.Address]protocol.PoolKey) []common.Address {
	addresses := make([]common.Address, 0, len(poolByAddress))
	for address := range poolByAddress {
		addresses = append(addresses, address)
	}
	return addresses
}
