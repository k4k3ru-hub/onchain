package sui

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	sharedCLI "github.com/k4k3ru-hub/cli/go"
	onchainSui "github.com/k4k3ru-hub/onchain/go/sui"
)

type stubClient struct{}

func (*stubClient) LatestCheckpointSequenceNumber(context.Context) (onchainSui.CheckpointSequenceNumber, error) {
	return 123456789, nil
}
func (*stubClient) CoinMetadata(context.Context, string) (*onchainSui.CoinMetadata, error) {
	coinType, _ := onchainSui.NormalizeMoveType("0x2::sui::SUI")
	return &onchainSui.CoinMetadata{CoinType: coinType, Name: "Sui", Symbol: "SUI", Decimals: 9, Supply: big.NewInt(100)}, nil
}
func (*stubClient) TransactionEffects(context.Context, onchainSui.TransactionDigest) (*onchainSui.TransactionEffects, error) {
	return &onchainSui.TransactionEffects{}, nil
}
func (*stubClient) TransactionDigests(context.Context, onchainSui.TransactionQuery) (onchainSui.TransactionDigestPage, error) {
	return onchainSui.TransactionDigestPage{}, nil
}

func TestLatestCheckpointSequenceNumber(t *testing.T) {
	var config onchainSui.RPCConfig
	command, err := NewCommand(func(_ context.Context, value onchainSui.RPCConfig) (Client, error) {
		config = value
		return &stubClient{}, nil
	})
	if err != nil {
		t.Fatalf("NewCommand() returned an unexpected error: %v", err)
	}
	commandLine := sharedCLI.NewCLIWithName("onchain", nil)
	if err := commandLine.Root().AddCommand(command); err != nil {
		t.Fatalf("AddCommand() returned an unexpected error: %v", err)
	}
	var output bytes.Buffer
	if err := commandLine.SetIO(strings.NewReader(""), &output, &bytes.Buffer{}); err != nil {
		t.Fatalf("SetIO() returned an unexpected error: %v", err)
	}
	if err := commandLine.RunArgs([]string{"sui", "get-latest-checkpoint-sequence-number", "-u", " https://example.com "}); err != nil {
		t.Fatalf("RunArgs() returned an unexpected error: %v", err)
	}
	if output.String() != "123456789\n" || config.URL != "https://example.com" {
		t.Fatalf("RunArgs() output=%q config=%+v", output.String(), config)
	}
}

func TestCoinCommands(t *testing.T) {
	command, err := NewCommand(func(context.Context, onchainSui.RPCConfig) (Client, error) { return &stubClient{}, nil })
	if err != nil {
		t.Fatalf("NewCommand() returned an unexpected error: %v", err)
	}
	commandLine := sharedCLI.NewCLIWithName("onchain", nil)
	if err := commandLine.Root().AddCommand(command); err != nil {
		t.Fatalf("AddCommand() returned an unexpected error: %v", err)
	}
	var output bytes.Buffer
	if err := commandLine.SetIO(strings.NewReader(""), &output, &bytes.Buffer{}); err != nil {
		t.Fatalf("SetIO() returned an unexpected error: %v", err)
	}
	if err := commandLine.RunArgs([]string{"sui", "get-coin-metadata", "0x2::sui::SUI", "--rpc-url", "https://example.com/graphql"}); err != nil {
		t.Fatalf("RunArgs(get-coin-metadata) returned an unexpected error: %v", err)
	}
	if !strings.Contains(output.String(), `"Symbol":"SUI"`) || !strings.Contains(output.String(), `"MetadataAddress":"0x`) {
		t.Fatalf("RunArgs(get-coin-metadata) output = %q", output.String())
	}
	output.Reset()
	if err := commandLine.RunArgs([]string{"sui", "get-transfers", "0x123", "0x2::sui::SUI", "--rpc-url", "https://example.com/graphql", "--first", "10"}); err != nil {
		t.Fatalf("RunArgs(get-transfers) returned an unexpected error: %v", err)
	}
	if !strings.Contains(output.String(), `"Transfers":[]`) {
		t.Fatalf("RunArgs(get-transfers) output = %q", output.String())
	}
}
