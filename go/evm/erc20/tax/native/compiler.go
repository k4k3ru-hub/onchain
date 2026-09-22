// Package native adapts application-owned native execution to token tax analysis.
package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/evm/erc20/tax"
)

type Runtime interface {
	Available(context.Context) ([]string, error)
	Compile(context.Context, string, json.RawMessage) (json.RawMessage, error)
}

type Compiler struct{ runtime Runtime }

var _ tax.Compiler = (*Compiler)(nil)

// NewCompiler adapts an explicitly composed, bounded native runtime. The caller
// owns its lifecycle, official compiler acquisition, cache and resource limits.
//
// Version:
//   - 2026-09-22: Added.
func NewCompiler(runtime Runtime) (*Compiler, error) {
	if runtime == nil {
		return nil, fmt.Errorf("failed to create native compiler adapter: runtime=null")
	}
	return &Compiler{runtime: runtime}, nil
}

// Available returns locally usable compilers without acquiring new versions.
//
// Version:
//   - 2026-09-22: Added.
func (c *Compiler) Available(ctx context.Context) ([]string, error) {
	versions, err := c.runtime.Available(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list native compilers: %w", err)
	}
	return versions, nil
}

// Compile preserves cancellation and maps native acquisition or compilation
// failures to an unverified candidate. It never infers tax zero from a failure.
//
// Version:
//   - 2026-09-22: Added.
func (c *Compiler) Compile(ctx context.Context, request tax.CompileRequest) (json.RawMessage, error) {
	out, err := c.runtime.Compile(ctx, request.Version, request.Input)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("failed to compile native candidate: %w", ctx.Err())
		}
		return nil, fmt.Errorf("failed to compile native candidate: %w: %w", tax.ErrCompilation, err)
	}
	return out, nil
}
