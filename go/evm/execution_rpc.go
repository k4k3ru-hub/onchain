package evm

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

type gasEstimator interface {
	EstimateGas(context.Context, ethereum.CallMsg) (uint64, error)
}

type pendingNonceProvider interface {
	PendingNonceAt(context.Context, common.Address) (uint64, error)
}

type gasTipCapSuggester interface {
	SuggestGasTipCap(context.Context) (*big.Int, error)
}

type FeeData struct {
	BaseFeePerGas              *big.Int
	SuggestedPriorityFeePerGas *big.Int
}

// EstimateGas estimates the gas required to execute an EVM call against the pending state.
//
// Parameters:
//   - ctx: Request context; nil uses context.Background.
//   - call: EVM call to estimate.
//
// Returns:
//   - Estimated gas limit.
//   - Estimation error.
//
// Version:
//   - 2026-09-05: Added.
func (c *HTTPClient) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	if c == nil {
		return 0, fmt.Errorf("failed to estimate evm gas: evm_http_client=null")
	}
	if c.gasEstimator == nil {
		return 0, fmt.Errorf("failed to estimate evm gas: gas_estimator=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	estimated, err := c.gasEstimator.EstimateGas(ctx, call)
	if err != nil {
		return 0, fmt.Errorf("failed to estimate evm gas: %w", err)
	}
	if estimated == 0 {
		return 0, fmt.Errorf("failed to estimate evm gas: estimated_gas=empty")
	}
	return estimated, nil
}

// PendingNonce gets an account nonce from the pending EVM state.
//
// Parameters:
//   - ctx: Request context; nil uses context.Background.
//   - account: Account whose nonce is requested.
//
// Returns:
//   - Pending account nonce.
//   - Retrieval error.
//
// Version:
//   - 2026-09-05: Added.
func (c *HTTPClient) PendingNonce(ctx context.Context, account common.Address) (uint64, error) {
	if c == nil {
		return 0, fmt.Errorf("failed to get pending evm nonce: evm_http_client=null")
	}
	if c.pendingNonceProvider == nil {
		return 0, fmt.Errorf("failed to get pending evm nonce: pending_nonce_provider=null")
	}
	if account == (common.Address{}) {
		return 0, fmt.Errorf("failed to get pending evm nonce: account=empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	nonce, err := c.pendingNonceProvider.PendingNonceAt(ctx, account)
	if err != nil {
		return 0, fmt.Errorf("failed to get pending evm nonce: %w", err)
	}
	return nonce, nil
}

// LatestFeeData gets the latest EIP-1559 base fee and suggested priority fee.
//
// Parameters:
//   - ctx: Request context; nil uses context.Background.
//
// Returns:
//   - Independent copies of the latest fee values.
//   - Retrieval error.
//
// Version:
//   - 2026-09-05: Added.
func (c *HTTPClient) LatestFeeData(ctx context.Context) (FeeData, error) {
	if c == nil {
		return FeeData{}, fmt.Errorf("failed to get latest evm fee data: evm_http_client=null")
	}
	if c.blockHeaderByNumberer == nil {
		return FeeData{}, fmt.Errorf("failed to get latest evm fee data: block_header_provider=null")
	}
	if c.gasTipCapSuggester == nil {
		return FeeData{}, fmt.Errorf("failed to get latest evm fee data: gas_tip_cap_suggester=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	header, err := c.blockHeaderByNumberer.HeaderByNumber(ctx, nil)
	if err != nil {
		return FeeData{}, fmt.Errorf("failed to get latest evm fee data: failed to get block header: %w", err)
	}
	if header == nil || header.BaseFee == nil || header.BaseFee.Sign() < 0 {
		return FeeData{}, fmt.Errorf("failed to get latest evm fee data: base_fee_per_gas=invalid")
	}
	tip, err := c.gasTipCapSuggester.SuggestGasTipCap(ctx)
	if err != nil {
		return FeeData{}, fmt.Errorf("failed to get latest evm fee data: failed to suggest priority fee: %w", err)
	}
	if tip == nil || tip.Sign() < 0 {
		return FeeData{}, fmt.Errorf("failed to get latest evm fee data: suggested_priority_fee_per_gas=invalid")
	}
	return FeeData{
		BaseFeePerGas:              new(big.Int).Set(header.BaseFee),
		SuggestedPriorityFeePerGas: new(big.Int).Set(tip),
	}, nil
}
