// log_test.go
package evm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var errTestRPC = errors.New("test rpc error")

type fakeLogFilterer struct {
	ctx  context.Context
	logs []types.Log
	err  error
}

func (f *fakeLogFilterer) FilterLogs(ctx context.Context, _ ethereum.FilterQuery) ([]types.Log, error) {
	f.ctx = ctx
	return f.logs, f.err
}

type fakeLogSubscriber struct {
	ctx          context.Context
	ch           chan<- types.Log
	subscription ethereum.Subscription
	err          error
}

func (f *fakeLogSubscriber) SubscribeFilterLogs(ctx context.Context, _ ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	f.ctx = ctx
	f.ch = ch
	return f.subscription, f.err
}

type fakeSubscription struct {
	errCh chan error
}

func (s *fakeSubscription) Err() <-chan error {
	return s.errCh
}

func (s *fakeSubscription) Unsubscribe() {
}

func TestNewHTTPClientRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		url       string
		wantError string
	}{
		{name: "empty", wantError: "failed to create evm http client: failed to validate evm rpc url: http_url=empty"},
		{name: "too long", url: strings.Repeat("a", 2049), wantError: "failed to create evm http client: failed to validate evm rpc url: http_url=too_long actual_length=2049 max_length=2048"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewHTTPClient(nil, HTTPConfig{URL: tt.url})
			if err == nil {
				t.Fatal("NewHTTPClient() error = nil, want error")
			}
			if err.Error() != tt.wantError {
				t.Errorf("NewHTTPClient() error = %q, want %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestNewWSClientRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		url       string
		wantError string
	}{
		{name: "empty", wantError: "failed to create evm ws client: failed to validate evm rpc url: ws_url=empty"},
		{name: "too long", url: strings.Repeat("a", 2049), wantError: "failed to create evm ws client: failed to validate evm rpc url: ws_url=too_long actual_length=2049 max_length=2048"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewWSClient(nil, WSConfig{URL: tt.url})
			if err == nil {
				t.Fatal("NewWSClient() error = nil, want error")
			}
			if err.Error() != tt.wantError {
				t.Errorf("NewWSClient() error = %q, want %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestComposeClientsComposeEVMDependencies(t *testing.T) {
	t.Parallel()

	httpETHClient := new(ethclient.Client)
	wsETHClient := new(ethclient.Client)
	httpClient := composeHTTPClient(HTTPConfig{}, httpETHClient)
	wsClient := composeWSClient(WSConfig{}, wsETHClient)

	if httpClient.ethClient != httpETHClient {
		t.Fatal("HTTP ETH client differs from composed client")
	}
	if wsClient.ethClient != wsETHClient {
		t.Fatal("WebSocket ETH client differs from composed client")
	}
	if httpClient.blockNumberer != httpETHClient {
		t.Fatal("HTTP block-number dependency was not composed")
	}
	if httpClient.logFilterer != httpETHClient {
		t.Fatal("HTTP log-filter dependency was not composed")
	}
	if httpClient.receiptProvider != httpETHClient {
		t.Fatal("HTTP transaction-receipt dependency was not composed")
	}
	if wsClient.logSubscriber != wsETHClient {
		t.Fatal("WebSocket log-subscription dependency was not composed")
	}
}

func TestFilterLogsRejectsNilReceiver(t *testing.T) {
	t.Parallel()

	var client *HTTPClient
	_, err := client.FilterLogs(context.Background(), ethereum.FilterQuery{})
	if err == nil {
		t.Fatal("FilterLogs() error = nil, want error")
	}
	if err.Error() != "failed to filter evm logs: evm_http_client=null" {
		t.Errorf("FilterLogs() error = %q", err.Error())
	}
}

func TestFilterLogsRejectsMissingHTTPClient(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{}
	_, err := client.FilterLogs(context.Background(), ethereum.FilterQuery{})
	if err == nil {
		t.Fatal("FilterLogs() error = nil, want error")
	}
	if err.Error() != "failed to filter evm logs: http_eth_client=null" {
		t.Errorf("FilterLogs() error = %q", err.Error())
	}
}

func TestFilterLogsHandlesNilContext(t *testing.T) {
	t.Parallel()

	filterer := &fakeLogFilterer{logs: []types.Log{}}
	client := &HTTPClient{logFilterer: filterer}
	logs, err := client.FilterLogs(nil, ethereum.FilterQuery{})
	if err != nil {
		t.Fatalf("FilterLogs() error = %v", err)
	}
	if logs == nil {
		t.Fatal("FilterLogs() logs = nil, want non-nil empty slice")
	}
	if filterer.ctx == nil {
		t.Fatal("FilterLogs() delegated context = nil, want context.Background()")
	}
}

func TestFilterLogsWrapsRPCError(t *testing.T) {
	t.Parallel()

	client := &HTTPClient{logFilterer: &fakeLogFilterer{err: errTestRPC}}
	_, err := client.FilterLogs(context.Background(), ethereum.FilterQuery{})
	if err == nil {
		t.Fatal("FilterLogs() error = nil, want error")
	}
	if !strings.HasPrefix(err.Error(), "failed to filter evm logs: ") {
		t.Errorf("FilterLogs() error = %q", err.Error())
	}
	if !errors.Is(err, errTestRPC) {
		t.Fatalf("errors.Is() = false, want wrapped error: %v", err)
	}
}

func TestSubscribeFilterLogsRejectsNilReceiver(t *testing.T) {
	t.Parallel()

	var client *WSClient
	_, err := client.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{}, make(chan types.Log))
	if err == nil {
		t.Fatal("SubscribeFilterLogs() error = nil, want error")
	}
	if err.Error() != "failed to subscribe evm logs: evm_ws_client=null" {
		t.Errorf("SubscribeFilterLogs() error = %q", err.Error())
	}
}

func TestSubscribeFilterLogsRejectsMissingWSClient(t *testing.T) {
	t.Parallel()

	client := &WSClient{}
	_, err := client.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{}, make(chan types.Log))
	if err == nil {
		t.Fatal("SubscribeFilterLogs() error = nil, want error")
	}
	if err.Error() != "failed to subscribe evm logs: ws_eth_client=null" {
		t.Errorf("SubscribeFilterLogs() error = %q", err.Error())
	}
}

func TestSubscribeFilterLogsRejectsNilChannel(t *testing.T) {
	t.Parallel()

	client := &WSClient{logSubscriber: &fakeLogSubscriber{}}
	_, err := client.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{}, nil)
	if err == nil {
		t.Fatal("SubscribeFilterLogs() error = nil, want error")
	}
	if err.Error() != "failed to subscribe evm logs: logs_channel=null" {
		t.Errorf("SubscribeFilterLogs() error = %q", err.Error())
	}
}

func TestSubscribeFilterLogsHandlesNilContext(t *testing.T) {
	t.Parallel()

	fakeSub := &fakeSubscription{errCh: make(chan error)}
	subscriber := &fakeLogSubscriber{subscription: fakeSub}
	client := &WSClient{logSubscriber: subscriber}
	logsCh := make(chan types.Log)
	subscription, err := client.SubscribeFilterLogs(nil, ethereum.FilterQuery{}, logsCh)
	if err != nil {
		t.Fatalf("SubscribeFilterLogs() error = %v", err)
	}
	if subscription == nil {
		t.Fatal("SubscribeFilterLogs() subscription = nil, want subscription")
	}
	if subscriber.ctx == nil {
		t.Fatal("SubscribeFilterLogs() delegated context = nil, want context.Background()")
	}
	if subscriber.ch != logsCh {
		t.Fatal("SubscribeFilterLogs() delegated channel differs from input")
	}
}

func TestSubscribeFilterLogsWrapsRPCError(t *testing.T) {
	t.Parallel()

	client := &WSClient{logSubscriber: &fakeLogSubscriber{err: errTestRPC}}
	_, err := client.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{}, make(chan types.Log))
	if err == nil {
		t.Fatal("SubscribeFilterLogs() error = nil, want error")
	}
	if !strings.HasPrefix(err.Error(), "failed to subscribe evm logs: ") {
		t.Errorf("SubscribeFilterLogs() error = %q", err.Error())
	}
	if !errors.Is(err, errTestRPC) {
		t.Fatalf("errors.Is() = false, want wrapped error: %v", err)
	}
}
