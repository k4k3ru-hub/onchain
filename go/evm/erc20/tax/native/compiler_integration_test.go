//go:build linux && integration

package native

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
	"github.com/k4k3ru-hub/onchain/go/evm/erc20/tax"
)

// This process bridge tests the released runtime executable without making the
// SDK depend on the server module. Production injects the in-process runtime.
type managerRuntime struct {
	path, cache, bundle, helper string
	compiled                    []string
	onlyLatest                  bool
}

func (m *managerRuntime) run(ctx context.Context, mode, version string, input json.RawMessage) ([]byte, error) {
	args := []string{"-mode", mode, "-cache", m.cache, "-bundle", m.bundle, "-helper", m.helper}
	if version != "" {
		args = append(args, "-version", version)
	}
	cmd := exec.CommandContext(ctx, m.path, args...)
	cmd.Env = []string{"LANG=C"}
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run native integration command: %w", err)
	}
	return out, nil
}

// Available queries only the supplied image's local artifacts.
//
// Version:
//   - 2026-09-22: Added.
func (m *managerRuntime) Available(ctx context.Context) ([]string, error) {
	out, err := m.run(ctx, "check", "", nil)
	if err != nil {
		return nil, err
	}
	var data struct {
		Versions []string `json:"versions"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("failed to decode native versions: %w", err)
	}
	if m.onlyLatest {
		return []string{"0.8.37+commit.f401782d"}, nil
	}
	return data.Versions, nil
}

// Compile invokes the formal manager and records version selection.
//
// Version:
//   - 2026-09-22: Added.
func (m *managerRuntime) Compile(ctx context.Context, version string, input json.RawMessage) (json.RawMessage, error) {
	m.compiled = append(m.compiled, version)
	return m.run(ctx, "compile", version, input)
}

type sourceFixture struct {
	Bundle tax.SourceBundle `json:"bundle"`
	Code   string           `json:"code"`
}

// Source supplies the saved, reviewed source bundle without external API traffic.
//
// Version:
//   - 2026-09-22: Added.
func (f *sourceFixture) Source(context.Context, tax.SourceRequest) (tax.SourceBundle, error) {
	return f.Bundle, nil
}

// CodeAtHash supplies a fixture runtime at the requested observation hash.
//
// Version:
//   - 2026-09-22: Added.
func (f *sourceFixture) CodeAtHash(context.Context, common.Address, common.Hash) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(f.Code, "0x"))
}

// TestNativeAnalyzerIntegration verifies standard and permit tokens, rejects tax
// overrides and unknown getters, and accepts a verified alternate compiler offline.
//
// Version:
//   - 2026-09-22: Added.
func TestNativeAnalyzerIntegration(t *testing.T) {
	manager := os.Getenv("SOLC_MANAGE_PATH")
	if manager == "" {
		t.Skip("SOLC_MANAGE_PATH is not configured")
	}
	for _, tc := range []struct {
		name                   string
		accepted, crossVersion bool
	}{
		{"simple", true, false}, {"permit", true, false}, {"taxed", false, false}, {"getter", false, false}, {"simple", true, true},
	} {
		t.Run(fmt.Sprintf("%s/cross=%t", tc.name, tc.crossVersion), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "testdata", tc.name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var fixture sourceFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatal(err)
			}
			if tc.crossVersion {
				old, err := os.ReadFile(filepath.Join("..", "testdata", "simple-0834-runtime.json"))
				if err != nil {
					t.Fatal(err)
				}
				var deployed struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(old, &deployed); err != nil {
					t.Fatal(err)
				}
				fixture.Code = deployed.Code
				fixture.Bundle.CompilerVersion = "0.8.34+commit.80d5c536"
			}
			runtime := &managerRuntime{path: manager, cache: t.TempDir(), bundle: os.Getenv("SOLC_BUNDLE_DIR"), helper: os.Getenv("SOLC_HELPER_PATH"), onlyLatest: tc.crossVersion}
			compiler, err := NewCompiler(runtime)
			if err != nil {
				t.Fatal(err)
			}
			analyzer, err := tax.NewAnalyzer(&fixture, &fixture, compiler)
			if err != nil {
				t.Fatal(err)
			}
			result, err := analyzer.Analyze(context.Background(), tax.Request{ChainID: evm.ChainIDBaseMainnet, Token: common.HexToAddress("0x1234"), BlockHash: common.HexToHash("0x5678")})
			if err != nil {
				t.Fatal(err)
			}
			if (result.Observation != nil) != tc.accepted {
				t.Fatalf("unexpected observation: %+v; compiled=%v", result, runtime.compiled)
			}
			if tc.accepted && (result.Observation.BuyRate == nil || *result.Observation.BuyRate != "0") {
				t.Fatal("tax zero not established")
			}
			if len(runtime.compiled) != 1 || runtime.compiled[0] != "0.8.37+commit.f401782d" {
				t.Fatalf("unexpected compiler acquisition: %v", runtime.compiled)
			}
			entries, err := os.ReadDir(runtime.cache)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() != ".lock" {
					t.Fatal("bundled verification acquired another compiler")
				}
			}
		})
	}
}
