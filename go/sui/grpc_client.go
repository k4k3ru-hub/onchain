package sui

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type GRPCConfig struct {
	URL string
}

type GRPCClient struct {
	config              GRPCConfig
	provider            eventSubscriptionProvider
	transactionProvider transactionSubscriptionProvider
	closer              grpcClientCloser
	closeOnce           sync.Once
	closeErr            error
}

type eventSubscriptionProvider interface {
	subscribeEvents(context.Context, LiveEventFilter) (liveEventReceiver, error)
}

type liveEventReceiver interface {
	Recv() (*EventNotification, error)
	Close()
}

type transactionSubscriptionProvider interface {
	subscribeTransactions(context.Context, Address) (liveTransactionReceiver, error)
}

type liveTransactionReceiver interface {
	Recv() (*TransactionNotification, error)
	Close()
}

type grpcClientCloser interface {
	Close() error
}

type grpcAdapter struct {
	client rpcv2.SubscriptionServiceClient
}

// NewGRPCClient creates a Sui gRPC streaming client.
//
// Parameters:
//   - ctx: construction context; nil uses context.Background.
//   - config: Sui gRPC configuration.
//
// Returns:
//   - Sui gRPC client.
//   - Client creation error.
//
// Version:
//   - 2026-08-23: Added.
func NewGRPCClient(ctx context.Context, config GRPCConfig) (*GRPCClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create sui grpc client: %w", err)
	}
	target, secure, serverName, err := resolveGRPCTarget(config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to create sui grpc client: %w", err)
	}
	var transportCredentials credentials.TransportCredentials
	if secure {
		transportCredentials = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName})
	} else {
		transportCredentials = insecure.NewCredentials()
	}
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		return nil, fmt.Errorf("failed to create sui grpc client: failed to create grpc connection: %w", err)
	}
	return composeGRPCClient(config, &grpcAdapter{client: rpcv2.NewSubscriptionServiceClient(connection)}, connection), nil
}

// Validate validates the Sui gRPC configuration.
func (c GRPCConfig) Validate() error {
	value := strings.TrimSpace(c.URL)
	if value == "" {
		return fmt.Errorf("failed to validate sui grpc config: grpc_url=empty")
	}
	if utf8.RuneCountInString(value) > 2048 {
		return fmt.Errorf("failed to validate sui grpc config: grpc_url=too_long actual_length=%d max_length=2048", utf8.RuneCountInString(value))
	}
	_, _, _, err := resolveGRPCTarget(value)
	return err
}

// Close closes the underlying Sui gRPC connection.
func (c *GRPCClient) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.closer != nil {
			c.closeErr = c.closer.Close()
		}
	})
	if c.closeErr != nil {
		return fmt.Errorf("failed to close sui grpc client: %w", c.closeErr)
	}
	return nil
}

func composeGRPCClient(config GRPCConfig, provider eventSubscriptionProvider, closer grpcClientCloser) *GRPCClient {
	client := &GRPCClient{config: config, provider: provider, closer: closer}
	if transactionProvider, ok := provider.(transactionSubscriptionProvider); ok {
		client.transactionProvider = transactionProvider
	}
	return client
}

func resolveGRPCTarget(value string) (string, bool, string, error) {
	trimmed := strings.TrimSpace(value)
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || (parsed.Path != "" && parsed.Path != "/") {
		return "", false, "", fmt.Errorf("failed to validate sui grpc config: grpc_url=invalid")
	}
	host := parsed.Host
	if parsed.Port() == "" {
		if parsed.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	return "dns:///" + host, parsed.Scheme == "https", parsed.Hostname(), nil
}
