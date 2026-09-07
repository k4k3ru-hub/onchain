package v3

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

type testSubscription struct {
	errs    chan error
	stopped chan struct{}
	once    sync.Once
}

func (s *testSubscription) Err() <-chan error { return s.errs }
func (s *testSubscription) Unsubscribe()      { s.once.Do(func() { close(s.stopped) }) }

type subscriptionWSRPC struct {
	logs   chan<- types.Log
	source ethereum.Subscription
}

func (r *subscriptionWSRPC) SubscribeFilterLogs(_ context.Context, _ ethereum.FilterQuery, logs chan<- types.Log) (ethereum.Subscription, error) {
	r.logs = logs
	return r.source, nil
}

func TestSubscribeSwaps(t *testing.T) {
	t.Parallel()
	config := testFactoryConfig(t)
	pool, err := config.PoolKeys[0].Address(config.Address, config.InitCodeHash)
	if err != nil {
		t.Fatal(err)
	}
	source := &testSubscription{errs: make(chan error, 1), stopped: make(chan struct{})}
	rpc := &subscriptionWSRPC{source: source}
	client, err := NewWSClient(WSClientParams{RPC: rpc, Factory: config})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := client.SubscribeSwaps(context.Background())
	if err != nil {
		t.Fatalf("SubscribeSwaps() error = %v", err)
	}
	rpc.logs <- testSwapLog(t, pool)
	select {
	case swap := <-subscription.Swaps():
		if swap.PoolKey.Fee != config.PoolKeys[0].Fee {
			t.Fatalf("swap = %+v", swap)
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
