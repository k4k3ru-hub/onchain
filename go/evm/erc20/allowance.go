package erc20

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	balanceOfMethodSelector = crypto.Keccak256([]byte("balanceOf(address)"))[:4]
	allowanceMethodSelector = crypto.Keccak256([]byte("allowance(address,address)"))[:4]
	approveMethodSelector   = crypto.Keccak256([]byte("approve(address,uint256)"))[:4]
)

// GetTokenBalance gets an owner's ERC20 balance at a block.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - token: configured ERC20 token contract address.
//   - owner: token owner address.
//   - blockNumber: block number; nil uses the latest block.
//
// Returns:
//   - Token balance in base units.
//   - Balance retrieval error.
//
// Version:
//   - 2026-09-09: Added.
func (c *Client) GetTokenBalance(ctx context.Context, token, owner common.Address, blockNumber *big.Int) (*big.Int, error) {
	if owner == (common.Address{}) {
		return nil, fmt.Errorf("failed to get erc20 token balance: owner=empty")
	}
	data := append(append([]byte(nil), balanceOfMethodSelector...), common.LeftPadBytes(owner.Bytes(), 32)...)
	result, err := c.callTokenUint256(ctx, token, blockNumber, data, "balanceOf")
	if err != nil {
		return nil, fmt.Errorf("failed to get erc20 token balance: %w", err)
	}
	return result, nil
}

// GetTokenAllowance gets an ERC20 allowance at a block.
//
// Parameters:
//   - ctx: request context; nil uses context.Background.
//   - token: configured ERC20 token contract address.
//   - owner: token owner address.
//   - spender: approved spender address.
//   - blockNumber: block number; nil uses the latest block.
//
// Returns:
//   - Available allowance in base units.
//   - Allowance retrieval error.
//
// Version:
//   - 2026-09-09: Added.
func (c *Client) GetTokenAllowance(ctx context.Context, token, owner, spender common.Address, blockNumber *big.Int) (*big.Int, error) {
	if owner == (common.Address{}) {
		return nil, fmt.Errorf("failed to get erc20 token allowance: owner=empty")
	}
	if spender == (common.Address{}) {
		return nil, fmt.Errorf("failed to get erc20 token allowance: spender=empty")
	}
	data := append(append([]byte(nil), allowanceMethodSelector...), common.LeftPadBytes(owner.Bytes(), 32)...)
	data = append(data, common.LeftPadBytes(spender.Bytes(), 32)...)
	result, err := c.callTokenUint256(ctx, token, blockNumber, data, "allowance")
	if err != nil {
		return nil, fmt.Errorf("failed to get erc20 token allowance: %w", err)
	}
	return result, nil
}

// EncodeApprove encodes an ERC20 approve call.
//
// Parameters:
//   - spender: approved spender address.
//   - amount: approval amount in base units; zero revokes approval.
//
// Returns:
//   - ABI-encoded approve calldata.
//   - Encoding error.
//
// Version:
//   - 2026-09-09: Added.
func EncodeApprove(spender common.Address, amount *big.Int) ([]byte, error) {
	if spender == (common.Address{}) {
		return nil, fmt.Errorf("failed to encode erc20 approve call: spender=empty")
	}
	if amount == nil {
		return nil, fmt.Errorf("failed to encode erc20 approve call: amount=null")
	}
	if amount.Sign() < 0 || amount.BitLen() > 256 {
		return nil, fmt.Errorf("failed to encode erc20 approve call: amount=out_of_range")
	}
	data := append(append([]byte(nil), approveMethodSelector...), common.LeftPadBytes(spender.Bytes(), 32)...)
	data = append(data, common.LeftPadBytes(amount.Bytes(), 32)...)
	return data, nil
}

func (c *Client) callTokenUint256(ctx context.Context, token common.Address, blockNumber *big.Int, data []byte, method string) (*big.Int, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to call erc20 uint256 method: client=null")
	}
	if c.httpClient == nil {
		return nil, fmt.Errorf("failed to call erc20 uint256 method: http_client=null")
	}
	if token == (common.Address{}) {
		return nil, fmt.Errorf("failed to call erc20 uint256 method: token=empty")
	}
	if !c.hasToken(token) {
		return nil, fmt.Errorf("failed to call erc20 uint256 method: token is not configured: token=%q", token.Hex())
	}
	if blockNumber != nil && blockNumber.Sign() < 0 {
		return nil, fmt.Errorf("failed to call erc20 uint256 method: block_number=out_of_range min_value=0")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := c.httpClient.CallContract(ctx, ethereum.CallMsg{To: &token, Data: append([]byte(nil), data...)}, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to call erc20 uint256 method: %w: method=%q token=%q", err, method, token.Hex())
	}
	if len(result) != 32 {
		return nil, fmt.Errorf("failed to call erc20 uint256 method: response=invalid: method=%q token=%q", method, token.Hex())
	}
	return new(big.Int).SetBytes(result), nil
}
