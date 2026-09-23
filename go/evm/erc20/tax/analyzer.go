package tax

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/k4k3ru-hub/onchain/go/evm"
	"github.com/k4k3ru-hub/onchain/go/evm/erc20/analysis"
)

// NewAnalyzer composes token tax analysis with explicitly supplied dependencies.
// Scheduling, retries, caching, observation timestamps and persistence belong to
// the caller. Construction neither reads environment variables nor starts work.
//
// Version:
//   - 2026-09-23: Share code verification without extending tax verdicts.
func NewAnalyzer(reader Reader, sources SourceProvider, compiler Compiler) (*Analyzer, error) {
	if reader == nil || sources == nil || compiler == nil {
		return nil, fmt.Errorf("failed to create token tax analyzer: dependency=null")
	}
	code, err := analysis.NewAnalyzer(reader, sourceAdapter{sources}, compilerAdapter{compiler})
	if err != nil {
		return nil, fmt.Errorf("failed to create token tax analyzer: %w", err)
	}
	return &Analyzer{reader: reader, sources: sources, compiler: compiler, code: code}, nil
}

// NewAnalyzerWithCode composes tax interpretation with a shared code verifier.
// The verifier must retain fixed-block code identity; it cannot supply an
// unverified remote verdict. Mutable permission state is outside tax analysis.
//
// Version:
//   - 2026-09-23: Added.
func NewAnalyzerWithCode(code CodeVerifier) (*Analyzer, error) {
	if code == nil {
		return nil, fmt.Errorf("failed to create token tax analyzer: code_verifier=null")
	}
	return &Analyzer{code: code}, nil
}

// Analyze evaluates one token at a caller-verified block hash on Base mainnet.
// Unsupported models return a nil observation and an internal reason. Retrieval
// failures return wrapped errors, allowing the caller to budget retries.
//
// Version:
//   - 2026-09-23: Share code verification without extending tax verdicts.
func (a *Analyzer) Analyze(ctx context.Context, request Request) (Result, error) {
	result := Result{BlockHash: request.BlockHash}
	if a == nil || a.code == nil {
		return result, fmt.Errorf("failed to analyze token taxes: analyzer=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("failed to analyze token taxes: %w", err)
	}
	if request.BlockHash == ([32]byte{}) {
		return result, fmt.Errorf("failed to analyze token taxes: block_hash=empty")
	}
	if request.ChainID != evm.ChainIDBaseMainnet {
		result.Reason = "unsupported_chain"
		return result, nil
	}
	if definition, ok := evm.LookupTokenMetadata(request.ChainID, request.Token); ok {
		if request.Token == ([20]byte{}) {
			result.Observation = zeroObservation("native_currency")
			result.Model = "native-v1"
			return result, nil
		}
		// This is trusted local reference data, never an RPC-returned symbol.
		if definition.Symbol == "USDC" {
			result.Reason = "trusted_token_skipped"
			return result, nil
		}
	}
	verified, err := a.code.Analyze(ctx, analysis.Request(request))
	result.CodeSHA256, result.Model, result.OriginalCompiler, result.UsedCompiler, result.Reason = verified.CodeSHA256, verified.Model, verified.OriginalCompiler, verified.UsedCompiler, verified.Reason
	if err != nil {
		return result, fmt.Errorf("failed to analyze token taxes: %w", err)
	}
	switch verified.Model {
	case "weth9-reviewed-runtime-v1", "taot-reviewed-runtime-v1", "openzeppelin-erc20-v5.5-v1", "openzeppelin-erc20-permit-v5.5-v1":
		result.Observation = zeroObservation("contract_analysis")
	default:
		if verified.Model != "" {
			result.Model = ""
			result.Reason = "unsupported_model"
		}
	}
	return result, nil
}

func zeroObservation(source string) *Observation {
	buy, sell, change, exemptions := "0", "0", false, false
	return &Observation{BuyRate: &buy, SellRate: &sell, CanChange: &change, HasExemptions: &exemptions, Source: source}
}

type sourceAdapter struct{ SourceProvider }

// Source adapts the existing provider without changing its public contract.
//
// Version:
//   - 2026-09-23: Added.
func (a sourceAdapter) Source(ctx context.Context, r analysis.SourceRequest) (analysis.SourceBundle, error) {
	b, err := a.SourceProvider.Source(ctx, SourceRequest(r))
	return analysis.SourceBundle(b), err
}

type compilerAdapter struct{ Compiler }

// Compile adapts the legacy compiler request to shared code analysis.
//
// Version:
//   - 2026-09-23: Added.
func (a compilerAdapter) Compile(ctx context.Context, r analysis.CompileRequest) (json.RawMessage, error) {
	return a.Compiler.Compile(ctx, CompileRequest(r))
}
