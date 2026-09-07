package slipstream

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
)

const (
	getPoolSignature    = "getPool(address,address,int24)"
	getSwapFeeSignature = "getSwapFee(address)"
)

type HTTPRPCClient interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

type FactoryConfig struct {
	Address common.Address
}

type FactoryClientParams struct {
	RPC     HTTPRPCClient
	Factory FactoryConfig
}

type FactoryClient struct {
	rpc     HTTPRPCClient
	factory common.Address
}

// NewFactoryClient creates a Slipstream CLFactory read client.
//
// Parameters:
//   - params: RPC dependency and Factory configuration.
//
// Returns:
//   - Factory read client.
//   - Client creation error.
//
// Version:
//   - 2026-08-30: Added.
func NewFactoryClient(params FactoryClientParams) (*FactoryClient, error) {
	if params.RPC == nil {
		return nil, fmt.Errorf("failed to create slipstream factory client: http_rpc_client=null")
	}
	if params.Factory.Address == (common.Address{}) {
		return nil, fmt.Errorf("failed to create slipstream factory client: factory=empty")
	}
	return &FactoryClient{rpc: params.RPC, factory: params.Factory.Address}, nil
}

// ResolvePool resolves a Slipstream pool through CLFactory.getPool.
//
// Parameters:
//   - ctx: Request context.
//   - key: Canonical Slipstream pool key.
//   - blockNumber: State block; nil uses latest.
//
// Returns:
//   - Resolved pool contract address.
//   - Resolution error, including when no pool exists.
//
// Version:
//   - 2026-08-30: Added.
func (c *FactoryClient) ResolvePool(ctx context.Context, key protocol.PoolKey, blockNumber *big.Int) (common.Address, error) {
	if c == nil {
		return common.Address{}, fmt.Errorf("failed to resolve slipstream pool: factory_client=null")
	}
	if err := key.Validate(); err != nil {
		return common.Address{}, fmt.Errorf("failed to resolve slipstream pool: %w", err)
	}
	if err := validateBlockNumber(blockNumber); err != nil {
		return common.Address{}, fmt.Errorf("failed to resolve slipstream pool: %w", err)
	}
	data := make([]byte, 4+32*3)
	copy(data[:4], methodSelector(getPoolSignature))
	copy(data[4+12:4+32], key.Token0.Address().Bytes())
	copy(data[4+32+12:4+64], key.Token1.Address().Bytes())
	new(big.Int).SetInt64(int64(key.TickSpacing)).FillBytes(data[4+64 : 4+96])

	response, err := c.call(ctx, data, blockNumber)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to resolve slipstream pool: failed to call cl factory getPool: %w", err)
	}
	pool, err := decodeAddress(response)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to resolve slipstream pool: %w", err)
	}
	if pool == (common.Address{}) {
		return common.Address{}, fmt.Errorf("failed to resolve slipstream pool: pool=empty")
	}
	return pool, nil
}

// GetSwapFee returns the current pool fee through CLFactory.getSwapFee.
//
// Parameters:
//   - ctx: Request context.
//   - pool: Slipstream pool contract address.
//   - blockNumber: State block; nil uses latest.
//
// Returns:
//   - Current fee in pips with 1e-6 denominator.
//   - Lookup error.
//
// Version:
//   - 2026-08-30: Added.
func (c *FactoryClient) GetSwapFee(ctx context.Context, pool common.Address, blockNumber *big.Int) (uint32, error) {
	if c == nil {
		return 0, fmt.Errorf("failed to get slipstream swap fee: factory_client=null")
	}
	if pool == (common.Address{}) {
		return 0, fmt.Errorf("failed to get slipstream swap fee: pool=empty")
	}
	if err := validateBlockNumber(blockNumber); err != nil {
		return 0, fmt.Errorf("failed to get slipstream swap fee: %w", err)
	}
	data := make([]byte, 4+32)
	copy(data[:4], methodSelector(getSwapFeeSignature))
	copy(data[4+12:], pool.Bytes())

	response, err := c.call(ctx, data, blockNumber)
	if err != nil {
		return 0, fmt.Errorf("failed to get slipstream swap fee: failed to call cl factory getSwapFee: %w", err)
	}
	fee, err := decodeUint24(response)
	if err != nil {
		return 0, fmt.Errorf("failed to get slipstream swap fee: %w", err)
	}
	return fee, nil
}

func (c *FactoryClient) call(ctx context.Context, data []byte, blockNumber *big.Int) ([]byte, error) {
	factory := c.factory
	return c.rpc.CallContract(ctx, ethereum.CallMsg{To: &factory, Data: data}, blockNumber)
}

func methodSelector(signature string) []byte { return crypto.Keccak256([]byte(signature))[:4] }

func validateBlockNumber(blockNumber *big.Int) error {
	if blockNumber != nil && blockNumber.Sign() < 0 {
		return fmt.Errorf("failed to validate block number: block_number=out_of_range")
	}
	return nil
}

func decodeAddress(response []byte) (common.Address, error) {
	if len(response) != 32 {
		return common.Address{}, fmt.Errorf("failed to decode cl factory address result: response_length=invalid actual_length=%d expected_length=32", len(response))
	}
	return common.BytesToAddress(response[12:]), nil
}

func decodeUint24(response []byte) (uint32, error) {
	if len(response) != 32 {
		return 0, fmt.Errorf("failed to decode cl factory uint24 result: response_length=invalid actual_length=%d expected_length=32", len(response))
	}
	value := new(big.Int).SetBytes(response)
	if value.BitLen() > 24 {
		return 0, fmt.Errorf("failed to decode cl factory uint24 result: value=out_of_range max_value=%d", uint32(1<<24-1))
	}
	return uint32(value.Uint64()), nil
}
