package main

import (
	"context"
	"strings"
	"testing"

	cliEVM "github.com/k4k3ru-hub/onchain/go/cli/evm"
	cliSolana "github.com/k4k3ru-hub/onchain/go/cli/solana"
	cliSui "github.com/k4k3ru-hub/onchain/go/cli/sui"
	onchainEVM "github.com/k4k3ru-hub/onchain/go/evm"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type stubEVMClient struct{}

func (*stubEVMClient) BlockNumber(context.Context) (uint64, error) { return 1, nil }
func (*stubEVMClient) Close()                                      {}

type stubSolanaClient struct{}

func (*stubSolanaClient) BlockHeight(context.Context) (onchainSolana.BlockHeight, error) {
	return 1, nil
}

type stubSuiClient struct{}

func (*stubSuiClient) LatestCheckpointSequenceNumber(context.Context) (onchainSui.CheckpointSequenceNumber, error) {
	return 1, nil
}
func (*stubSuiClient) CoinMetadata(context.Context, string) (*onchainSui.CoinMetadata, error) {
	return nil, nil
}
func (*stubSuiClient) TransactionEffects(context.Context, onchainSui.TransactionDigest) (*onchainSui.TransactionEffects, error) {
	return nil, nil
}
func (*stubSuiClient) TransactionDigests(context.Context, onchainSui.TransactionQuery) (onchainSui.TransactionDigestPage, error) {
	return onchainSui.TransactionDigestPage{}, nil
}

func TestNewCLIComposesChainCommands(t *testing.T) {
	evmFactory := func(context.Context, onchainEVM.HTTPConfig) (cliEVM.Client, error) {
		return &stubEVMClient{}, nil
	}
	solanaFactory := func(context.Context, onchainSolana.RPCConfig) (cliSolana.Client, error) {
		return &stubSolanaClient{}, nil
	}
	suiFactory := func(context.Context, onchainSui.RPCConfig) (cliSui.Client, error) {
		return &stubSuiClient{}, nil
	}
	commandLine, err := newCLI(evmFactory, solanaFactory, suiFactory)
	if err != nil {
		t.Fatalf("newCLI() returned an unexpected error: %v", err)
	}
	if commandLine == nil || commandLine.Root() == nil {
		t.Fatal("newCLI() returned an incomplete CLI")
	}

	for _, args := range [][]string{{"evm", "get-block-number"}, {"solana", "get-block-height"}, {"sui", "get-latest-checkpoint-sequence-number"}} {
		err := commandLine.RunArgs(args)
		if err == nil || !strings.Contains(err.Error(), "rpc_url=empty") {
			t.Fatalf("RunArgs(%v) error = %v, want chain command rpc_url validation", args, err)
		}
	}
}
