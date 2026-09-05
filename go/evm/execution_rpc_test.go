package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var errTestExecutionRPC = errors.New("test execution rpc error")

type executionRPCStub struct {
	ctx       context.Context
	call      ethereum.CallMsg
	account   common.Address
	estimated uint64
	nonce     uint64
	header    *types.Header
	tip       *big.Int
	err       error
}

// EstimateGas returns a configured gas estimate.
//
// Version:
//   - 2026-09-05: Added.
func (s *executionRPCStub) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	s.ctx, s.call = ctx, call
	return s.estimated, s.err
}

// PendingNonceAt returns a configured pending nonce.
//
// Version:
//   - 2026-09-05: Added.
func (s *executionRPCStub) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	s.ctx, s.account = ctx, account
	return s.nonce, s.err
}

// HeaderByNumber returns a configured block header.
//
// Version:
//   - 2026-09-05: Added.
func (s *executionRPCStub) HeaderByNumber(ctx context.Context, _ *big.Int) (*types.Header, error) {
	s.ctx = ctx
	return s.header, s.err
}

// SuggestGasTipCap returns a configured priority fee.
//
// Version:
//   - 2026-09-05: Added.
func (s *executionRPCStub) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	s.ctx = ctx
	return s.tip, s.err
}

func TestEstimateGasDelegates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	call := ethereum.CallMsg{Data: []byte{1}}
	stub := &executionRPCStub{estimated: 123}
	client := &HTTPClient{gasEstimator: stub}
	result, err := client.EstimateGas(ctx, call)
	if err != nil {
		t.Fatalf("EstimateGas() error = %v", err)
	}
	if result != 123 || stub.ctx != ctx || len(stub.call.Data) != 1 {
		t.Fatalf("EstimateGas() result/delegation = %d/%#v", result, stub.call)
	}
}

func TestEstimateGasRejectsInvalidResultAndWrapsError(t *testing.T) {
	t.Parallel()
	client := &HTTPClient{gasEstimator: &executionRPCStub{}}
	if _, err := client.EstimateGas(nil, ethereum.CallMsg{}); err == nil {
		t.Fatal("EstimateGas() zero result error = nil")
	}
	client.gasEstimator = &executionRPCStub{err: errTestExecutionRPC}
	if _, err := client.EstimateGas(nil, ethereum.CallMsg{}); err == nil || !errors.Is(err, errTestExecutionRPC) {
		t.Fatalf("EstimateGas() error = %v", err)
	}
}

func TestPendingNonceDelegates(t *testing.T) {
	t.Parallel()
	account := common.HexToAddress("0x0000000000000000000000000000000000000001")
	stub := &executionRPCStub{nonce: 7}
	client := &HTTPClient{pendingNonceProvider: stub}
	result, err := client.PendingNonce(nil, account)
	if err != nil {
		t.Fatalf("PendingNonce() error = %v", err)
	}
	if result != 7 || stub.account != account || stub.ctx == nil {
		t.Fatalf("PendingNonce() result/delegation = %d/%s", result, stub.account)
	}
}

func TestPendingNonceRejectsEmptyAccount(t *testing.T) {
	t.Parallel()
	client := &HTTPClient{pendingNonceProvider: &executionRPCStub{}}
	if _, err := client.PendingNonce(nil, common.Address{}); err == nil {
		t.Fatal("PendingNonce() error = nil")
	}
}

func TestLatestFeeDataReturnsCopies(t *testing.T) {
	t.Parallel()
	baseFee, tip := big.NewInt(10), big.NewInt(2)
	stub := &executionRPCStub{header: &types.Header{BaseFee: baseFee}, tip: tip}
	client := &HTTPClient{blockHeaderByNumberer: stub, gasTipCapSuggester: stub}
	result, err := client.LatestFeeData(nil)
	if err != nil {
		t.Fatalf("LatestFeeData() error = %v", err)
	}
	if result.BaseFeePerGas.Cmp(baseFee) != 0 || result.SuggestedPriorityFeePerGas.Cmp(tip) != 0 {
		t.Fatalf("LatestFeeData() = %+v", result)
	}
	baseFee.SetInt64(20)
	tip.SetInt64(3)
	if result.BaseFeePerGas.Int64() != 10 || result.SuggestedPriorityFeePerGas.Int64() != 2 {
		t.Fatalf("LatestFeeData() aliases provider values = %+v", result)
	}
}

func TestLatestFeeDataRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	client := &HTTPClient{blockHeaderByNumberer: &executionRPCStub{header: &types.Header{}}, gasTipCapSuggester: &executionRPCStub{tip: big.NewInt(1)}}
	if _, err := client.LatestFeeData(nil); err == nil {
		t.Fatal("LatestFeeData() missing base fee error = nil")
	}
	client.blockHeaderByNumberer = &executionRPCStub{header: &types.Header{BaseFee: big.NewInt(1)}}
	client.gasTipCapSuggester = &executionRPCStub{}
	if _, err := client.LatestFeeData(nil); err == nil {
		t.Fatal("LatestFeeData() missing priority fee error = nil")
	}
}
