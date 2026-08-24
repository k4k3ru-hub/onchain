package solana

import (
	"context"
	"fmt"
	"strings"

	sharedCLI "github.com/k4k3ru-hub/cli/go"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

const (
	optionRPCURL     = "rpc-url"
	optionCommitment = "commitment"
)

// Client provides the Solana operations required by the CLI.
type Client interface {
	BlockHeight(context.Context) (onchainSolana.BlockHeight, error)
}

// ClientFactory creates a Solana client for a CLI execution.
type ClientFactory func(context.Context, onchainSolana.RPCConfig) (Client, error)

// NewCommand creates the Solana command tree.
//
// Parameters:
//   - factory: Solana client factory.
//
// Returns:
//   - Configured Solana command.
//   - Command construction error.
//
// Version:
//   - 2026-08-22: Added.
func NewCommand(factory ClientFactory) (*sharedCLI.Command, error) {
	if factory == nil {
		return nil, fmt.Errorf("failed to create solana command: client_factory=null")
	}
	solanaCommand := sharedCLI.NewCommand("solana")
	solanaCommand.SetUsage("Access Solana JSON-RPC methods.")
	getBlockHeightCommand, err := newGetBlockHeightCommand(factory)
	if err != nil {
		return nil, fmt.Errorf("failed to create solana get-block-height command: %w", err)
	}
	if err := solanaCommand.AddCommand(getBlockHeightCommand); err != nil {
		return nil, fmt.Errorf("failed to add solana get-block-height command: %w", err)
	}
	return solanaCommand, nil
}

func newGetBlockHeightCommand(factory ClientFactory) (*sharedCLI.Command, error) {
	command := sharedCLI.NewCommand("get-block-height")
	command.SetUsage("Return the current block height using Solana's getBlockHeight RPC method.")
	command.SetAction(getBlockHeightAction(factory))
	if err := command.SetArgumentCount(0, 0); err != nil {
		return nil, fmt.Errorf("failed to configure get-block-height arguments: %w", err)
	}
	if err := command.AddOption(optionRPCURL, sharedCLI.Option{Alias: "u", Description: "Solana JSON-RPC endpoint URL."}); err != nil {
		return nil, fmt.Errorf("failed to add rpc-url option: %w", err)
	}
	if err := command.AddOption(optionCommitment, sharedCLI.Option{Alias: "c", DefaultValue: string(onchainSolana.CommitmentFinalized), Description: "Solana commitment: processed, confirmed, or finalized."}); err != nil {
		return nil, fmt.Errorf("failed to add commitment option: %w", err)
	}
	return command, nil
}

func getBlockHeightAction(factory ClientFactory) sharedCLI.CommandFunc {
	return func(commandContext *sharedCLI.Context) error {
		rpcURL, ok := commandContext.Option(optionRPCURL)
		if !ok || strings.TrimSpace(rpcURL.Value) == "" {
			return fmt.Errorf("failed to get solana block height: rpc_url=empty")
		}
		commitment, ok := commandContext.Option(optionCommitment)
		if !ok {
			return fmt.Errorf("failed to get solana block height: commitment=missing")
		}
		requestContext := context.Background()
		client, err := factory(requestContext, onchainSolana.RPCConfig{URL: strings.TrimSpace(rpcURL.Value), Commitment: onchainSolana.Commitment(commitment.Value)})
		if err != nil {
			return fmt.Errorf("failed to get solana block height: failed to create rpc client: %w", err)
		}
		if client == nil {
			return fmt.Errorf("failed to get solana block height: rpc_client=null")
		}
		blockHeight, err := client.BlockHeight(requestContext)
		if err != nil {
			return fmt.Errorf("failed to get solana block height: %w", err)
		}
		if _, err := fmt.Fprintln(commandContext.Output(), blockHeight); err != nil {
			return fmt.Errorf("failed to output solana block height: %w", err)
		}
		return nil
	}
}
