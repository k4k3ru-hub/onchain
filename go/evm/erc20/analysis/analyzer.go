package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/k4k3ru-hub/onchain/go/evm"
)

// NewAnalyzer composes token code analysis with explicitly supplied dependencies.
// Scheduling, retries, caching, observation timestamps and persistence belong to
// the caller. Construction neither reads environment variables nor starts work.
//
// Version:
//   - 2026-09-23: Extracted shared code verification.
func NewAnalyzer(reader Reader, sources SourceProvider, compiler Compiler) (*Analyzer, error) {
	if reader == nil || sources == nil || compiler == nil {
		return nil, fmt.Errorf("failed to create token code analyzer: dependency=null")
	}
	return &Analyzer{reader: reader, sources: sources, compiler: compiler}, nil
}

// Analyze evaluates one token at a caller-verified block hash on Base mainnet.
// Unsupported models return an empty model and an internal reason. Retrieval
// failures return wrapped errors, allowing the caller to budget retries.
//
// Version:
//   - 2026-09-23: Recognize reviewed complete OniAgent runtime alongside shared code verification.
func (a *Analyzer) Analyze(ctx context.Context, request Request) (Result, error) {
	result := Result{BlockHash: request.BlockHash}
	if a == nil || a.reader == nil || a.sources == nil || a.compiler == nil {
		return result, fmt.Errorf("failed to analyze token code: analyzer=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("failed to analyze token code: %w", err)
	}
	if request.BlockHash == ([32]byte{}) {
		return result, fmt.Errorf("failed to analyze token code: block_hash=empty")
	}
	if request.ChainID != evm.ChainIDBaseMainnet {
		result.Reason = "unsupported_chain"
		return result, nil
	}
	code, err := a.reader.CodeAtHash(ctx, request.Token, request.BlockHash)
	if err != nil {
		return result, fmt.Errorf("failed to analyze token code: %w", err)
	}
	if len(code) == 0 {
		result.Reason = "empty_code"
		return result, nil
	}
	if len(code) > 128*1024 {
		result.Reason = "unsupported_code_size"
		return result, nil
	}
	digest := sha256.Sum256(code)
	result.CodeSHA256 = hex.EncodeToString(digest[:])
	if model, ok := reviewedRuntimes[result.CodeSHA256]; ok {
		result.Model = model
		return result, nil
	}
	bundle, err := a.sources.Source(ctx, SourceRequest{ChainID: request.ChainID, Token: request.Token})
	if errors.Is(err, ErrUnsupported) {
		result.Reason = "source_unavailable"
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("failed to analyze token code: %w", err)
	}
	input, err := prepareInput(bundle)
	if err != nil {
		result.Reason = "unsupported_source"
		return result, nil
	}
	result.OriginalCompiler = bundle.CompilerVersion
	available, err := a.compiler.Available(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to analyze token code: %w", err)
	}
	versions := candidateVersions(bundle.CompilerVersion, available)
	for _, version := range versions {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("failed to analyze token code: %w", err)
		}
		output, err := a.compiler.Compile(ctx, CompileRequest{Version: version, Input: append([]byte(nil), input...)})
		if errors.Is(err, ErrUnsupported) || errors.Is(err, ErrCompilation) {
			continue
		}
		if err != nil {
			return result, fmt.Errorf("failed to analyze token code: %w", err)
		}
		artifact, sources, err := parseOutput(output, bundle)
		if errors.Is(err, ErrCompilation) {
			continue
		}
		if err != nil {
			return result, fmt.Errorf("failed to analyze token code: %w", err)
		}
		model, allowedImmutables, ok := recognizeModel(bundle, sources)
		if !ok {
			result.Reason = "unsupported_model"
			return result, nil
		}
		if !matchRuntime(artifact, code, allowedImmutables) {
			continue
		}
		result.Model, result.UsedCompiler = model, version
		return result, nil
	}
	result.Reason = "unverified_runtime"
	return result, nil
}

func candidateVersions(original string, available []string) []string {
	unique := map[string]bool{}
	var versions []string
	for _, version := range available {
		if !validVersion(version) || unique[version] {
			continue
		}
		unique[version] = true
		if version == original {
			return []string{original}
		}
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersion(versions[i], versions[j]) > 0 })
	// At most two locally available alternatives, then the source's exact version.
	if len(versions) > 2 {
		versions = versions[:2]
	}
	return append(versions, original)
}
