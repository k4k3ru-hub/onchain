package solana

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	sharedCLI "github.com/k4k3ru-hub/cli/go"
	onchainSolana "github.com/k4k3ru-hub/onchain/go/solana"
)

type stubClient struct {
	blockHeight onchainSolana.BlockHeight
	err         error
}

func (c *stubClient) BlockHeight(context.Context) (onchainSolana.BlockHeight, error) {
	return c.blockHeight, c.err
}

func TestGetBlockHeight(t *testing.T) {
	var capturedConfig onchainSolana.RPCConfig
	factory := func(_ context.Context, config onchainSolana.RPCConfig) (Client, error) {
		capturedConfig = config
		return &stubClient{blockHeight: 123456789}, nil
	}
	command, err := NewCommand(factory)
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
	if err := commandLine.RunArgs([]string{"solana", "get-block-height", "-u", " https://api.devnet.solana.com ", "-c", "confirmed"}); err != nil {
		t.Fatalf("RunArgs() returned an unexpected error: %v", err)
	}
	if output.String() != "123456789\n" {
		t.Fatalf("RunArgs() output = %q, want %q", output.String(), "123456789\n")
	}
	if capturedConfig.URL != "https://api.devnet.solana.com" || capturedConfig.Commitment != onchainSolana.CommitmentConfirmed {
		t.Fatalf("RunArgs() config = %+v, want trimmed URL and confirmed commitment", capturedConfig)
	}
}

func TestGetBlockHeightWrapsClientError(t *testing.T) {
	wantErr := errors.New("rpc unavailable")
	command, err := NewCommand(func(context.Context, onchainSolana.RPCConfig) (Client, error) {
		return &stubClient{err: wantErr}, nil
	})
	if err != nil {
		t.Fatalf("NewCommand() returned an unexpected error: %v", err)
	}
	commandLine := sharedCLI.NewCLIWithName("onchain", nil)
	if err := commandLine.Root().AddCommand(command); err != nil {
		t.Fatalf("AddCommand() returned an unexpected error: %v", err)
	}
	err = commandLine.RunArgs([]string{"solana", "get-block-height", "-u", "https://example.com"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunArgs() error = %v, want wrapped error %v", err, wantErr)
	}
}

func TestNewCommandRejectsNilFactory(t *testing.T) {
	_, err := NewCommand(nil)
	if err == nil || !strings.Contains(err.Error(), "client_factory=null") {
		t.Fatalf("NewCommand() error = %v, want client_factory=null", err)
	}
}
