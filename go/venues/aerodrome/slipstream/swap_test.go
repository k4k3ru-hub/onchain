package slipstream

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/k4k3ru-hub/onchain/go/venues/aerodrome/slipstream/protocol"
)

type swapFilterTestRPC struct {
	logs  []types.Log
	err   error
	query ethereum.FilterQuery
}

// FilterLogs returns the configured log fixture.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func (r *swapFilterTestRPC) FilterLogs(_ context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	r.query = query
	return r.logs, r.err
}

type swapTestSubscription struct {
	errs    chan error
	stopped chan struct{}
	once    sync.Once
}

// Err exposes the fixture subscription error channel.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func (s *swapTestSubscription) Err() <-chan error { return s.errs }

// Unsubscribe signals fixture subscription closure.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func (s *swapTestSubscription) Unsubscribe() {
	s.once.Do(func() { close(s.stopped) })
}

type swapSubscriptionTestRPC struct {
	logs   chan<- types.Log
	query  ethereum.FilterQuery
	source ethereum.Subscription
	err    error
}

// SubscribeFilterLogs records the query and attaches the fixture subscription.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func (r *swapSubscriptionTestRPC) SubscribeFilterLogs(_ context.Context, query ethereum.FilterQuery, logs chan<- types.Log) (ethereum.Subscription, error) {
	r.logs = logs
	r.query = query
	return r.source, r.err
}

// TestDecodeSwapLog verifies decode swap log in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestDecodeSwapLog(t *testing.T) {
	t.Parallel()
	pool := common.HexToAddress("0x0000000000000000000000000000000000000005")
	eventLog := swapTestLog(t, pool)
	swap, err := DecodeSwapLog(eventLog)
	if err != nil {
		t.Fatalf("DecodeSwapLog() error = %v", err)
	}
	if swap.PoolAddress != pool || swap.Amount0.Cmp(big.NewInt(100)) != 0 || swap.Amount1.Cmp(big.NewInt(-90)) != 0 || swap.SqrtPriceX96.Cmp(big.NewInt(123)) != 0 || swap.Liquidity.Cmp(big.NewInt(456)) != 0 || swap.Tick != -7 || swap.Sender != common.HexToAddress("0x03") || swap.BlockNumber != eventLog.BlockNumber || swap.LogIndex != eventLog.Index {
		t.Fatalf("DecodeSwapLog() = %+v", swap)
	}
}

// TestDecodeSwapLogRejectsInvalidValues verifies decode swap log rejects invalid values in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestDecodeSwapLogRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	valid := swapTestLog(t, common.HexToAddress("0x05"))
	tests := []types.Log{
		{},
		{Address: valid.Address},
		{Address: valid.Address, Topics: []common.Hash{{}, valid.Topics[1], valid.Topics[2]}, Data: valid.Data},
		{Address: valid.Address, Topics: valid.Topics, Data: nil},
	}
	for _, eventLog := range tests {
		if _, err := DecodeSwapLog(eventLog); err == nil {
			t.Fatalf("DecodeSwapLog(%+v) error = nil", eventLog)
		}
	}
}

// TestSwapFilterClientFilterSwaps verifies swap filter client filter swaps in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestSwapFilterClientFilterSwaps(t *testing.T) {
	t.Parallel()
	sources := swapTestSources(t)
	rpc := &swapFilterTestRPC{logs: []types.Log{swapTestLog(t, sources[0].PoolAddress), swapTestLog(t, sources[1].PoolAddress)}}
	client, err := NewSwapFilterClient(SwapFilterClientParams{RPC: rpc, Sources: sources})
	if err != nil {
		t.Fatalf("NewSwapFilterClient() error = %v", err)
	}
	fromBlock := big.NewInt(10)
	toBlock := big.NewInt(20)
	swaps, err := client.FilterSwaps(context.Background(), fromBlock, toBlock)
	if err != nil {
		t.Fatalf("FilterSwaps() error = %v", err)
	}
	if len(swaps) != 2 || swaps[0].PoolKey.TickSpacing != 50 || swaps[1].PoolKey.TickSpacing != 100 {
		t.Fatalf("FilterSwaps() = %+v", swaps)
	}
	if rpc.query.FromBlock != fromBlock || rpc.query.ToBlock != toBlock || len(rpc.query.Addresses) != 2 || len(rpc.query.Topics) != 1 || rpc.query.Topics[0][0] != swapEventSignatureHash() {
		t.Fatalf("FilterSwaps() query = %+v", rpc.query)
	}
}

// TestSwapFilterClientRejectsInvalidInputs verifies swap filter client rejects invalid inputs in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestSwapFilterClientRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	if _, err := NewSwapFilterClient(SwapFilterClientParams{}); err == nil {
		t.Fatal("NewSwapFilterClient() error = nil")
	}
	if _, err := NewSwapFilterClient(SwapFilterClientParams{RPC: &swapFilterTestRPC{}}); err == nil {
		t.Fatal("NewSwapFilterClient() error = nil for empty sources")
	}
	client, err := NewSwapFilterClient(SwapFilterClientParams{RPC: &swapFilterTestRPC{}, Sources: swapTestSources(t)})
	if err != nil {
		t.Fatalf("NewSwapFilterClient() error = %v", err)
	}
	if _, err := client.FilterSwaps(context.Background(), big.NewInt(2), big.NewInt(1)); err == nil {
		t.Fatal("FilterSwaps() error = nil")
	}
}

// TestSwapFilterClientPropagatesErrors verifies swap filter client propagates errors in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestSwapFilterClientPropagatesErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("filter failed")
	client, err := NewSwapFilterClient(SwapFilterClientParams{RPC: &swapFilterTestRPC{err: wantErr}, Sources: swapTestSources(t)})
	if err != nil {
		t.Fatalf("NewSwapFilterClient() error = %v", err)
	}
	if _, err := client.FilterSwaps(context.Background(), nil, nil); !errors.Is(err, wantErr) {
		t.Fatalf("FilterSwaps() error = %v", err)
	}
}

// TestSwapSubscriberSubscribeSwaps verifies swap subscriber subscribe swaps in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestSwapSubscriberSubscribeSwaps(t *testing.T) {
	t.Parallel()
	sources := swapTestSources(t)
	source := &swapTestSubscription{errs: make(chan error, 1), stopped: make(chan struct{})}
	rpc := &swapSubscriptionTestRPC{source: source}
	subscriber, err := NewSwapSubscriber(SwapSubscriberParams{RPC: rpc, Sources: sources})
	if err != nil {
		t.Fatalf("NewSwapSubscriber() error = %v", err)
	}
	subscription, err := subscriber.SubscribeSwaps(context.Background())
	if err != nil {
		t.Fatalf("SubscribeSwaps() error = %v", err)
	}
	if len(rpc.query.Addresses) != 2 || rpc.query.Topics[0][0] != swapEventSignatureHash() {
		t.Fatalf("SubscribeSwaps() query = %+v", rpc.query)
	}
	rpc.logs <- swapTestLog(t, sources[1].PoolAddress)
	select {
	case swap := <-subscription.Swaps():
		if swap.PoolKey.TickSpacing != 100 {
			t.Fatalf("Swap = %+v", swap)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Swap")
	}
	subscription.Unsubscribe()
	select {
	case <-source.stopped:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unsubscribe")
	}
}

// TestSwapSubscriberForwardsTerminalError verifies swap subscriber forwards terminal error in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestSwapSubscriberForwardsTerminalError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("subscription failed")
	source := &swapTestSubscription{errs: make(chan error, 1), stopped: make(chan struct{})}
	rpc := &swapSubscriptionTestRPC{source: source}
	subscriber, err := NewSwapSubscriber(SwapSubscriberParams{RPC: rpc, Sources: swapTestSources(t)})
	if err != nil {
		t.Fatalf("NewSwapSubscriber() error = %v", err)
	}
	subscription, err := subscriber.SubscribeSwaps(nil)
	if err != nil {
		t.Fatalf("SubscribeSwaps() error = %v", err)
	}
	source.errs <- wantErr
	select {
	case err := <-subscription.Err():
		if !errors.Is(err, wantErr) {
			t.Fatalf("Err() = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for terminal error")
	}
}

// TestSwapSourcesRejectDuplicates verifies swap sources reject duplicates in the migrated SDK.
//
// Version:
//   - 2026-09-07: Migrated to onchain.
func TestSwapSourcesRejectDuplicates(t *testing.T) {
	t.Parallel()
	sources := swapTestSources(t)
	sources[1].PoolAddress = sources[0].PoolAddress
	if _, _, err := buildSwapSources(sources); err == nil {
		t.Fatal("buildSwapSources() error = nil")
	}
}

func swapTestSources(t *testing.T) []SwapSource {
	t.Helper()
	weth := protocol.NewCurrency(common.HexToAddress("0x4200000000000000000000000000000000000006"))
	usdc := protocol.NewCurrency(common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"))
	current, err := protocol.NewPoolKey(weth, usdc, 50)
	if err != nil {
		t.Fatalf("NewPoolKey() error = %v", err)
	}
	legacy, err := protocol.NewPoolKey(weth, usdc, 100)
	if err != nil {
		t.Fatalf("NewPoolKey() error = %v", err)
	}
	return []SwapSource{
		{PoolAddress: common.HexToAddress("0x3FE04A59Ebd38cF06080a6F60a98D124eb59392A"), PoolKey: current},
		{PoolAddress: common.HexToAddress("0xb2cc224c1c9feE385f8ad6a55b4d94E92359DC59"), PoolKey: legacy},
	}
}

func swapTestLog(t *testing.T, pool common.Address) types.Log {
	t.Helper()
	data, err := abi.Arguments{
		{Type: swapTestABIType(t, "int256")},
		{Type: swapTestABIType(t, "int256")},
		{Type: swapTestABIType(t, "uint160")},
		{Type: swapTestABIType(t, "uint128")},
		{Type: swapTestABIType(t, "int24")},
	}.Pack(big.NewInt(100), big.NewInt(-90), big.NewInt(123), big.NewInt(456), big.NewInt(-7))
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	return types.Log{
		Address:     pool,
		Topics:      []common.Hash{swapEventSignatureHash(), common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x03").Bytes(), 32)), common.BytesToHash(common.LeftPadBytes(common.HexToAddress("0x04").Bytes(), 32))},
		Data:        data,
		BlockNumber: 10,
		BlockHash:   common.HexToHash("0x06"),
		TxHash:      common.HexToHash("0x07"),
		TxIndex:     1,
		Index:       2,
	}
}

func swapTestABIType(t *testing.T, name string) abi.Type {
	t.Helper()
	value, err := abi.NewType(name, "", nil)
	if err != nil {
		t.Fatalf("NewType(%q) error = %v", name, err)
	}
	return value
}
