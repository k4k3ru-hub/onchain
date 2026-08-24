package evm

import (
	"context"
	"fmt"
	"strings"

	sharedCLI "github.com/k4k3ru-hub/cli/go"
	onchainEVM "github.com/k4k3ru-hub/onchain/go/evm"
)

const optionRPCURL = "rpc-url"

// Client provides the EVM operations required by the CLI.
type Client interface {
	BlockNumber(context.Context) (uint64, error)
	Close()
}

// ClientFactory creates an EVM client for a CLI execution.
type ClientFactory func(context.Context, onchainEVM.HTTPConfig) (Client, error)

// NewCommand creates the EVM command tree.
//
// Parameters:
//   - factory: EVM client factory.
//
// Returns:
//   - Configured EVM command.
//   - Command construction error.
//
// Version:
//   - 2026-08-22: Added.
func NewCommand(factory ClientFactory) (*sharedCLI.Command, error) {
	if factory == nil {
		return nil, fmt.Errorf("failed to create evm command: client_factory=null")
	}
	evmCommand := sharedCLI.NewCommand("evm")
	evmCommand.SetUsage("Access EVM JSON-RPC methods.")
	getBlockNumberCommand, err := newGetBlockNumberCommand(factory)
	if err != nil {
		return nil, fmt.Errorf("failed to create evm get-block-number command: %w", err)
	}
	if err := evmCommand.AddCommand(getBlockNumberCommand); err != nil {
		return nil, fmt.Errorf("failed to add evm get-block-number command: %w", err)
	}
	return evmCommand, nil
}

func newGetBlockNumberCommand(factory ClientFactory) (*sharedCLI.Command, error) {
	command := sharedCLI.NewCommand("get-block-number")
	command.SetUsage("Return the latest block number using EVM's eth_blockNumber RPC method.")
	command.SetAction(getBlockNumberAction(factory))
	if err := command.SetArgumentCount(0, 0); err != nil {
		return nil, fmt.Errorf("failed to configure get-block-number arguments: %w", err)
	}
	if err := command.AddOption(optionRPCURL, sharedCLI.Option{Alias: "u", Description: "EVM HTTP JSON-RPC endpoint URL."}); err != nil {
		return nil, fmt.Errorf("failed to add rpc-url option: %w", err)
	}
	return command, nil
}

func getBlockNumberAction(factory ClientFactory) sharedCLI.CommandFunc {
	return func(commandContext *sharedCLI.Context) error {
		rpcURL, ok := commandContext.Option(optionRPCURL)
		if !ok || strings.TrimSpace(rpcURL.Value) == "" {
			return fmt.Errorf("failed to get evm block number: rpc_url=empty")
		}
		requestContext := context.Background()
		client, err := factory(requestContext, onchainEVM.HTTPConfig{URL: strings.TrimSpace(rpcURL.Value)})
		if err != nil {
			return fmt.Errorf("failed to get evm block number: failed to create http client: %w", err)
		}
		if client == nil {
			return fmt.Errorf("failed to get evm block number: http_client=null")
		}
		defer client.Close()
		blockNumber, err := client.BlockNumber(requestContext)
		if err != nil {
			return fmt.Errorf("failed to get evm block number: %w", err)
		}
		if _, err := fmt.Fprintln(commandContext.Output(), blockNumber); err != nil {
			return fmt.Errorf("failed to output evm block number: %w", err)
		}
		return nil
	}
}
