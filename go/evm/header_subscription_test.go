package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

type fakeHeadSubscriber struct {
	headers chan<- *types.Header
	err     error
	sub     *countedHeaderSubscription
}

func (f *fakeHeadSubscriber) SubscribeNewHead(_ context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	f.headers = ch
	return f.sub, f.err
}

type countedHeaderSubscription struct {
	errors chan error
	closes int
}

func (s *countedHeaderSubscription) Err() <-chan error { return s.errors }
func (s *countedHeaderSubscription) Unsubscribe()      { s.closes++ }

func newTestHeaderSubscription(t *testing.T) (*HeaderSubscription, *fakeHeadSubscriber) {
	t.Helper()
	fake := &fakeHeadSubscriber{sub: &countedHeaderSubscription{errors: make(chan error, 1)}}
	sub, err := (&WSClient{headSubscriber: fake}).SubscribeHeaders(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	return sub, fake
}

func TestHeaderSubscriptionOwnsPayloadAndCloses(t *testing.T) {
	sub, fake := newTestHeaderSubscription(t)
	header := &types.Header{Number: big.NewInt(123), Time: 456, BaseFee: big.NewInt(789)}
	fake.headers <- header
	got, err := sub.Recv(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 123 || got.Timestamp != 456 || got.Hash != header.Hash() {
		t.Fatalf("unexpected header: %+v", got)
	}
	header.BaseFee.SetInt64(1)
	if got.BaseFeePerGas.Int64() != 789 {
		t.Fatal("base fee aliases transport payload")
	}
	fake.headers <- header
	sub.Close()
	sub.Close()
	if fake.sub.closes != 1 {
		t.Fatalf("unsubscribe calls = %d", fake.sub.closes)
	}
	if _, err := sub.Recv(nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("closed receive: %v", err)
	}
}

func TestHeaderSubscriptionInterruptsBlockedReceive(t *testing.T) {
	sub, _ := newTestHeaderSubscription(t)
	done := make(chan error, 1)
	go func() { _, err := sub.Recv(nil); done <- err }()
	sub.Close()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("receive did not stop")
	}
}

func TestHeaderSubscriptionPropagatesTransportFailure(t *testing.T) {
	sub, fake := newTestHeaderSubscription(t)
	fake.headers <- &types.Header{Number: big.NewInt(123)}
	fake.sub.errors <- errTestRPC
	if _, err := sub.Recv(nil); !errors.Is(err, errTestRPC) {
		t.Fatalf("receive error: %v", err)
	}
	_, err := (&WSClient{headSubscriber: &fakeHeadSubscriber{err: errTestRPC}}).SubscribeHeaders(nil)
	if !errors.Is(err, errTestRPC) {
		t.Fatalf("subscribe error: %v", err)
	}
}

func TestHeaderSubscriptionRejectsInvalidHeader(t *testing.T) {
	for _, header := range []*types.Header{nil, {}, {Number: big.NewInt(-1)}} {
		sub, fake := newTestHeaderSubscription(t)
		fake.headers <- header
		if _, err := sub.Recv(nil); err == nil {
			t.Fatal("accepted invalid header")
		}
	}
}

func TestHeaderSubscriptionReportsClosedTransport(t *testing.T) {
	sub, fake := newTestHeaderSubscription(t)
	close(fake.sub.errors)
	if _, err := sub.Recv(nil); err == nil {
		t.Fatal("accepted closed transport")
	}
}
