package sui

import (
	"context"
	"encoding/json"
	"fmt"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	sharedCLI "github.com/k4k3ru-hub/cli/go"
	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
	suiCoin "github.com/k4k3ru-hub/onchain/go/sui/coin"
)

const (
	optionRPCURL  = "rpc-url"
	optionGRPCURL = "grpc-url"
	optionFirst   = "first"
	optionAfter   = "after"
	optionOnce    = "once"
)

type Client interface {
	LatestCheckpointSequenceNumber(context.Context) (onchainSui.CheckpointSequenceNumber, error)
	CoinMetadata(context.Context, string) (*onchainSui.CoinMetadata, error)
	TransactionEffects(context.Context, onchainSui.TransactionDigest) (*onchainSui.TransactionEffects, error)
	TransactionDigests(context.Context, onchainSui.TransactionQuery) (onchainSui.TransactionDigestPage, error)
}

type GRPCClient interface {
	SubscribeTransactions(context.Context, onchainSui.Address) (*onchainSui.TransactionSubscription, error)
	Close() error
}

type ClientFactory func(context.Context, onchainSui.RPCConfig) (Client, error)
type GRPCClientFactory func(context.Context, onchainSui.GRPCConfig) (GRPCClient, error)

// NewCommand creates the Sui command tree.
//
// Version:
//   - 2026-08-23: Added coin metadata, transfer history, and transfer subscription commands.
//   - 2026-08-22: Added.
func NewCommand(factory ClientFactory, grpcFactories ...GRPCClientFactory) (*sharedCLI.Command, error) {
	if factory == nil {
		return nil, fmt.Errorf("failed to create sui command: client_factory=null")
	}
	if len(grpcFactories) > 1 {
		return nil, fmt.Errorf("failed to create sui command: grpc_client_factories=too_long actual_length=%d max_length=1", len(grpcFactories))
	}
	var grpcFactory GRPCClientFactory
	if len(grpcFactories) == 1 {
		grpcFactory = grpcFactories[0]
	}
	root := sharedCLI.NewCommand("sui")
	root.SetUsage("Access Sui GraphQL and gRPC operations.")
	builders := []func() (*sharedCLI.Command, error){
		func() (*sharedCLI.Command, error) { return checkpointCommand(factory) },
		func() (*sharedCLI.Command, error) { return metadataCommand(factory) },
		func() (*sharedCLI.Command, error) { return transfersCommand(factory) },
		func() (*sharedCLI.Command, error) { return subscribeCommand(factory, grpcFactory) },
	}
	for _, build := range builders {
		command, err := build()
		if err != nil {
			return nil, fmt.Errorf("failed to create sui subcommand: %w", err)
		}
		if err := root.AddCommand(command); err != nil {
			return nil, fmt.Errorf("failed to add sui subcommand: %w", err)
		}
	}
	return root, nil
}

func checkpointCommand(factory ClientFactory) (*sharedCLI.Command, error) {
	command := sharedCLI.NewCommand("get-latest-checkpoint-sequence-number")
	command.SetUsage("Return the latest checkpoint sequence number.")
	command.SetAction(checkpointAction(factory))
	if err := command.SetArgumentCount(0, 0); err != nil {
		return nil, err
	}
	return command, addOption(command, optionRPCURL, "u", "Sui GraphQL RPC endpoint URL.", "", false)
}

func metadataCommand(factory ClientFactory) (*sharedCLI.Command, error) {
	command := sharedCLI.NewCommand("get-coin-metadata")
	command.SetUsage("Return coin metadata. Usage: get-coin-metadata <coin-type> --rpc-url <url>.")
	command.SetAction(metadataAction(factory))
	if err := command.SetArgumentCount(1, 1); err != nil {
		return nil, err
	}
	return command, addOption(command, optionRPCURL, "u", "Sui GraphQL RPC endpoint URL.", "", false)
}

func transfersCommand(factory ClientFactory) (*sharedCLI.Command, error) {
	command := sharedCLI.NewCommand("get-transfers")
	command.SetUsage("Return transfers. Usage: get-transfers <address> <coin-type> --rpc-url <url>.")
	command.SetAction(transfersAction(factory))
	if err := command.SetArgumentCount(2, 2); err != nil {
		return nil, err
	}
	for _, option := range []struct{ name, alias, description, defaultValue string }{
		{optionRPCURL, "u", "Sui GraphQL RPC endpoint URL.", ""}, {optionFirst, "", "Maximum transaction count.", "50"}, {optionAfter, "", "GraphQL transaction cursor.", ""},
	} {
		if err := addOption(command, option.name, option.alias, option.description, option.defaultValue, false); err != nil {
			return nil, err
		}
	}
	return command, nil
}

func subscribeCommand(factory ClientFactory, grpcFactory GRPCClientFactory) (*sharedCLI.Command, error) {
	command := sharedCLI.NewCommand("subscribe-transfers")
	command.SetUsage("Stream transfers. Usage: subscribe-transfers <address> <coin-type> --rpc-url <url> --grpc-url <url>.")
	command.SetAction(subscribeAction(factory, grpcFactory))
	if err := command.SetArgumentCount(2, 2); err != nil {
		return nil, err
	}
	if err := addOption(command, optionGRPCURL, "u", "Sui gRPC endpoint URL.", "", false); err != nil {
		return nil, err
	}
	if err := addOption(command, optionRPCURL, "", "Sui GraphQL RPC endpoint URL.", "", false); err != nil {
		return nil, err
	}
	if err := addOption(command, optionOnce, "", "Exit after the first transfer.", "", true); err != nil {
		return nil, err
	}
	return command, nil
}

func checkpointAction(factory ClientFactory) sharedCLI.CommandFunc {
	return func(ctx *sharedCLI.Context) error {
		url, err := requiredOption(ctx, optionRPCURL)
		if err != nil {
			return fmt.Errorf("failed to get latest sui checkpoint sequence number: %w", err)
		}
		client, err := createClient(context.Background(), factory, url)
		if err != nil {
			return fmt.Errorf("failed to get latest sui checkpoint sequence number: %w", err)
		}
		value, err := client.LatestCheckpointSequenceNumber(context.Background())
		if err != nil {
			return fmt.Errorf("failed to get latest sui checkpoint sequence number: %w", err)
		}
		if _, err := fmt.Fprintln(ctx.Output(), value); err != nil {
			return fmt.Errorf("failed to output latest sui checkpoint sequence number: %w", err)
		}
		return nil
	}
}

func metadataAction(factory ClientFactory) sharedCLI.CommandFunc {
	return func(ctx *sharedCLI.Context) error {
		url, err := requiredOption(ctx, optionRPCURL)
		if err != nil {
			return fmt.Errorf("failed to get sui coin metadata: %w", err)
		}
		coinType := ctx.Arguments()[0]
		client, err := createCoinClient(context.Background(), factory, url, coinType)
		if err != nil {
			return fmt.Errorf("failed to get sui coin metadata: %w", err)
		}
		value, err := client.Metadata(context.Background(), coinType)
		if err != nil {
			return fmt.Errorf("failed to get sui coin metadata: %w", err)
		}
		return outputJSON(ctx, value, "sui coin metadata")
	}
}

func transfersAction(factory ClientFactory) sharedCLI.CommandFunc {
	return func(ctx *sharedCLI.Context) error {
		url, err := requiredOption(ctx, optionRPCURL)
		if err != nil {
			return fmt.Errorf("failed to get sui coin transfers: %w", err)
		}
		args := ctx.Arguments()
		address, err := onchainSui.ParseAddress(args[0])
		if err != nil {
			return fmt.Errorf("failed to get sui coin transfers: address=invalid")
		}
		first, err := integerOption(ctx, optionFirst)
		if err != nil {
			return fmt.Errorf("failed to get sui coin transfers: %w", err)
		}
		client, err := createCoinClient(context.Background(), factory, url, args[1])
		if err != nil {
			return fmt.Errorf("failed to get sui coin transfers: %w", err)
		}
		after, _ := ctx.Option(optionAfter)
		value, err := client.Transfers(context.Background(), suiCoin.TransferQuery{Address: address, CoinType: args[1], First: first, After: after.Value})
		if err != nil {
			return fmt.Errorf("failed to get sui coin transfers: %w", err)
		}
		return outputJSON(ctx, value, "sui coin transfers")
	}
}

func subscribeAction(factory ClientFactory, grpcFactory GRPCClientFactory) sharedCLI.CommandFunc {
	return func(cliContext *sharedCLI.Context) error {
		if grpcFactory == nil {
			return fmt.Errorf("failed to subscribe sui coin transfers: grpc_client_factory=null")
		}
		url, err := requiredOption(cliContext, optionGRPCURL)
		if err != nil {
			return fmt.Errorf("failed to subscribe sui coin transfers: %w", err)
		}
		args := cliContext.Arguments()
		rpcURL, err := requiredOption(cliContext, optionRPCURL)
		if err != nil {
			return fmt.Errorf("failed to subscribe sui coin transfers: %w", err)
		}
		address, err := onchainSui.ParseAddress(args[0])
		if err != nil {
			return fmt.Errorf("failed to subscribe sui coin transfers: address=invalid")
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		coinClient, err := createCoinClient(ctx, factory, rpcURL, args[1])
		if err != nil {
			return fmt.Errorf("failed to subscribe sui coin transfers: %w", err)
		}
		grpcClient, err := grpcFactory(ctx, onchainSui.GRPCConfig{URL: url})
		if err != nil {
			return fmt.Errorf("failed to subscribe sui coin transfers: failed to create grpc client: %w", err)
		}
		if grpcClient == nil {
			return fmt.Errorf("failed to subscribe sui coin transfers: grpc_client=null")
		}
		defer grpcClient.Close()
		subscriber, err := suiCoin.NewGRPCTransferSubscriber(coinClient, grpcClient)
		if err != nil {
			return fmt.Errorf("failed to subscribe sui coin transfers: %w", err)
		}
		coinClient.WithTransferSubscriber(subscriber)
		subscription, err := coinClient.SubscribeTransfers(ctx, suiCoin.TransferSubscriptionFilter{Address: address, CoinType: args[1]})
		if err != nil {
			return fmt.Errorf("failed to subscribe sui coin transfers: %w", err)
		}
		defer subscription.Close()
		once, _ := cliContext.Option(optionOnce)
		for {
			transfer, err := subscription.Recv(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("failed to subscribe sui coin transfers: %w", err)
			}
			if err := outputJSON(cliContext, transfer, "sui coin transfer"); err != nil {
				return err
			}
			if once.IsSet {
				return nil
			}
		}
	}
}

func createClient(ctx context.Context, factory ClientFactory, url string) (Client, error) {
	client, err := factory(ctx, onchainSui.RPCConfig{URL: url})
	if err != nil {
		return nil, fmt.Errorf("failed to create rpc client: %w", err)
	}
	if client == nil {
		return nil, fmt.Errorf("failed to create rpc client: rpc_client=null")
	}
	return client, nil
}
func createCoinClient(ctx context.Context, factory ClientFactory, url, coinType string) (*suiCoin.Client, error) {
	provider, err := createClient(ctx, factory, url)
	if err != nil {
		return nil, err
	}
	client, err := suiCoin.NewClient(provider, []string{coinType})
	if err != nil {
		return nil, fmt.Errorf("failed to create coin client: %w", err)
	}
	return client, nil
}
func requiredOption(ctx *sharedCLI.Context, name string) (string, error) {
	value, ok := ctx.Option(name)
	if !ok || strings.TrimSpace(value.Value) == "" {
		return "", fmt.Errorf("failed to validate command option: %s=empty", strings.ReplaceAll(name, "-", "_"))
	}
	return strings.TrimSpace(value.Value), nil
}
func integerOption(ctx *sharedCLI.Context, name string) (int, error) {
	value, ok := ctx.Option(name)
	if !ok {
		return 0, fmt.Errorf("failed to validate command option: %s=missing", name)
	}
	number, err := strconv.Atoi(value.Value)
	if err != nil || number < 1 || number > 100 {
		return 0, fmt.Errorf("failed to validate command option: %s=out_of_range min_value=1 max_value=100", name)
	}
	return number, nil
}
func outputJSON(ctx *sharedCLI.Context, value any, object string) error {
	encoder := json.NewEncoder(ctx.Output())
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("failed to output %s: %w", object, err)
	}
	return nil
}
func addOption(command *sharedCLI.Command, name, alias, description, defaultValue string, flag bool) error {
	if err := command.AddOption(name, sharedCLI.Option{Alias: alias, Description: description, DefaultValue: defaultValue, IsFlag: flag}); err != nil {
		return fmt.Errorf("failed to add %s option: %w", name, err)
	}
	return nil
}
