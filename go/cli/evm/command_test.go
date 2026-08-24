package evm

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	sharedCLI "github.com/k4k3ru-hub/cli/go"
	onchainEVM "github.com/k4k3ru-hub/onchain/go/evm"
)

type stubClient struct {
	blockNumber uint64
	err         error
	closed      bool
}

func (c *stubClient) BlockNumber(context.Context) (uint64, error) {
	return c.blockNumber, c.err
}

func (c *stubClient) Close() {
	c.closed = true
}

func TestGetBlockNumber(t *testing.T) {
	client := &stubClient{blockNumber: 123456789}
	var capturedConfig onchainEVM.HTTPConfig
	factory := func(_ context.Context, config onchainEVM.HTTPConfig) (Client, error) {
		capturedConfig = config
		return client, nil
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
	if err := commandLine.RunArgs([]string{"evm", "get-block-number", "--rpc-url", " https://example.com "}); err != nil {
		t.Fatalf("RunArgs() returned an unexpected error: %v", err)
	}
	if output.String() != "123456789\n" {
		t.Fatalf("RunArgs() output = %q, want %q", output.String(), "123456789\n")
	}
	if capturedConfig.URL != "https://example.com" {
		t.Fatalf("RunArgs() rpc URL = %q, want trimmed URL", capturedConfig.URL)
	}
	if !client.closed {
		t.Fatal("RunArgs() did not close the EVM client")
	}
}

func TestGetBlockNumberWrapsClientError(t *testing.T) {
	wantErr := errors.New("rpc unavailable")
	command, err := NewCommand(func(context.Context, onchainEVM.HTTPConfig) (Client, error) {
		return &stubClient{err: wantErr}, nil
	})
	if err != nil {
		t.Fatalf("NewCommand() returned an unexpected error: %v", err)
	}
	commandLine := sharedCLI.NewCLIWithName("onchain", nil)
	if err := commandLine.Root().AddCommand(command); err != nil {
		t.Fatalf("AddCommand() returned an unexpected error: %v", err)
	}
	err = commandLine.RunArgs([]string{"evm", "get-block-number", "-u", "https://example.com"})
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
