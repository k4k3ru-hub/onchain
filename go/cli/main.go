package main

import (
	"context"
	"fmt"
	"io"
	"os"

	sharedCLI "github.com/k4k3ru-hub/cli/go"
	cliEVM "github.com/k4k3ru-hub/onchain/go/cli/evm"
	cliSolana "github.com/k4k3ru-hub/onchain/go/cli/solana"
	cliSui "github.com/k4k3ru-hub/onchain/go/cli/sui"
	onchainEVM "github.com/k4k3ru-hub/onchain/go/evm"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, newEVMHTTPClient, newSolanaRPCClient, newSuiRPCClient, newSuiGRPCClient); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, input io.Reader, output io.Writer, errorOutput io.Writer, evmFactory cliEVM.ClientFactory, solanaFactory cliSolana.ClientFactory, suiFactory cliSui.ClientFactory, suiGRPCFactory cliSui.GRPCClientFactory) error {
	commandLine, err := newCLI(evmFactory, solanaFactory, suiFactory, suiGRPCFactory)
	if err != nil {
		return fmt.Errorf("failed to create onchain cli: %w", err)
	}
	if err := commandLine.SetIO(input, output, errorOutput); err != nil {
		return fmt.Errorf("failed to configure onchain cli io: %w", err)
	}
	if err := commandLine.RunArgs(args); err != nil {
		return fmt.Errorf("failed to run onchain cli: %w", err)
	}
	return nil
}

func newCLI(evmFactory cliEVM.ClientFactory, solanaFactory cliSolana.ClientFactory, suiFactory cliSui.ClientFactory, suiGRPCFactories ...cliSui.GRPCClientFactory) (*sharedCLI.CLI, error) {
	commandLine := sharedCLI.NewCLIWithName("onchain", nil)
	evmCommand, err := cliEVM.NewCommand(evmFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to create evm command: %w", err)
	}
	if err := commandLine.Root().AddCommand(evmCommand); err != nil {
		return nil, fmt.Errorf("failed to add evm command: %w", err)
	}
	solanaCommand, err := cliSolana.NewCommand(solanaFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to create solana command: %w", err)
	}
	if err := commandLine.Root().AddCommand(solanaCommand); err != nil {
		return nil, fmt.Errorf("failed to add solana command: %w", err)
	}
	suiCommand, err := cliSui.NewCommand(suiFactory, suiGRPCFactories...)
	if err != nil {
		return nil, fmt.Errorf("failed to create sui command: %w", err)
	}
	if err := commandLine.Root().AddCommand(suiCommand); err != nil {
		return nil, fmt.Errorf("failed to add sui command: %w", err)
	}
	return commandLine, nil
}

func newSuiGRPCClient(ctx context.Context, config onchainSui.GRPCConfig) (cliSui.GRPCClient, error) {
	client, err := onchainSui.NewGRPCClient(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create sui grpc client: %w", err)
	}
	return client, nil
}

func newSuiRPCClient(ctx context.Context, config onchainSui.RPCConfig) (cliSui.Client, error) {
	client, err := onchainSui.NewRPCClient(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create sui rpc client: %w", err)
	}
	return client, nil
}

func newEVMHTTPClient(ctx context.Context, config onchainEVM.HTTPConfig) (cliEVM.Client, error) {
	client, err := onchainEVM.NewHTTPClient(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create evm http client: %w", err)
	}
	return client, nil
}

func newSolanaRPCClient(ctx context.Context, config onchainSolana.RPCConfig) (cliSolana.Client, error) {
	client, err := onchainSolana.NewRPCClient(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create solana rpc client: %w", err)
	}
	return client, nil
}
