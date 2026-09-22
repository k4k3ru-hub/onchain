package native

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/k4k3ru-hub/onchain/go/evm/erc20/tax"
)

type fakeRuntime struct {
	err     error
	request tax.CompileRequest
}

// Available supplies a local compiler list.
//
// Version:
//   - 2026-09-22: Added.
func (r *fakeRuntime) Available(context.Context) ([]string, error) {
	return []string{"0.8.37+commit.f401782d"}, nil
}

// Compile records unchanged input and returns an injected result.
//
// Version:
//   - 2026-09-22: Added.
func (r *fakeRuntime) Compile(_ context.Context, version string, input json.RawMessage) (json.RawMessage, error) {
	r.request = tax.CompileRequest{Version: version, Input: input}
	return json.RawMessage(`{}`), r.err
}

// TestCompilerComposition verifies delegation and native failure classification.
//
// Version:
//   - 2026-09-22: Added.
func TestCompilerComposition(t *testing.T) {
	if _, err := NewCompiler(nil); err == nil {
		t.Fatal("nil runtime accepted")
	}
	f := &fakeRuntime{}
	c, err := NewCompiler(f)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := c.Available(context.Background())
	if err != nil || len(versions) != 1 {
		t.Fatal(versions, err)
	}
	req := tax.CompileRequest{Version: versions[0], Input: json.RawMessage(`{"language":"Solidity"}`)}
	if _, err := c.Compile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if f.request.Version != req.Version || string(f.request.Input) != string(req.Input) {
		t.Fatal("request changed")
	}
	sentinel := errors.New("download failed")
	f.err = sentinel
	if _, err := c.Compile(context.Background(), req); !errors.Is(err, tax.ErrCompilation) || !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	f.err = context.DeadlineExceeded
	if _, err := c.Compile(context.Background(), req); !errors.Is(err, tax.ErrCompilation) {
		t.Fatal("per-compiler timeout not a candidate failure", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Compile(ctx, req); !errors.Is(err, context.Canceled) || errors.Is(err, tax.ErrCompilation) {
		t.Fatal("job cancellation lost", err)
	}
}
