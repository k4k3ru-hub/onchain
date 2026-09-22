// Package tax observes token transfer taxes from reviewed code and source models.
// It does not assess tradability, token safety, liquidity protection or DEX fees.
package tax

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

// ModelVersion identifies the reviewed analysis rules for persisted evidence.
const ModelVersion = "evm-token-taxes-v1"

var (
	ErrUnsupported = errors.New("unsupported token tax analysis")
	ErrCompilation = errors.New("solidity compilation failed")
)

type Request struct {
	ChainID evm.ChainID
	Token   common.Address
	// BlockHash must identify the caller's verified observation block.
	BlockHash common.Hash
}

type Observation struct {
	BuyRate       *string
	SellRate      *string
	CanChange     *bool
	HasExemptions *bool
	Source        string
}

// Result keeps internal analysis evidence separate from the public observation.
// A nil observation is unknown or intentionally skipped; it is never tax zero.
type Result struct {
	Observation      *Observation
	BlockHash        common.Hash
	CodeSHA256       string
	Model            string
	OriginalCompiler string
	UsedCompiler     string
	Reason           string
}

type Reader interface {
	CodeAtHash(context.Context, common.Address, common.Hash) ([]byte, error)
}

type SourceRequest struct {
	ChainID evm.ChainID
	Token   common.Address
}

type SourceBundle struct {
	CompilerVersion string
	ContractFile    string
	ContractName    string
	Input           json.RawMessage
}

type SourceProvider interface {
	Source(context.Context, SourceRequest) (SourceBundle, error)
}

type CompileRequest struct {
	Version string
	Input   json.RawMessage
}

// Compiler is supplied by the application's composition root. Available returns
// locally usable versions without fetching compilers. Compile may acquire a
// requested missing version; it must verify the executable and enforce limits.
// Output must come from this exact input and version, including AST and runtime.
type Compiler interface {
	Available(context.Context) ([]string, error)
	Compile(context.Context, CompileRequest) (json.RawMessage, error)
}

type Analyzer struct {
	reader   Reader
	sources  SourceProvider
	compiler Compiler
}
