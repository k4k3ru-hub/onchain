package tax

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type fixture struct {
	Bundle SourceBundle    `json:"bundle"`
	Code   string          `json:"code"`
	Output json.RawMessage `json:"output"`
}
type fakeDependencies struct {
	fixture                        fixture
	reads                          int
	sourceCalls                    int
	versions                       []string
	compiled                       []string
	readErr, sourceErr, compileErr error
	compile                        func(CompileRequest) (json.RawMessage, error)
	hash                           common.Hash
}

// CodeAtHash records the observation position and returns fixture code.
//
// Version:
//   - 2026-09-22: Added.
func (f *fakeDependencies) CodeAtHash(_ context.Context, _ common.Address, hash common.Hash) ([]byte, error) {
	f.reads++
	f.hash = hash
	if f.readErr != nil {
		return nil, f.readErr
	}
	return hex.DecodeString(strings.TrimPrefix(f.fixture.Code, "0x"))
}

// Source returns a prepared source bundle.
//
// Version:
//   - 2026-09-22: Added.
func (f *fakeDependencies) Source(context.Context, SourceRequest) (SourceBundle, error) {
	f.sourceCalls++
	return f.fixture.Bundle, f.sourceErr
}

// Available returns only locally usable fixture versions.
//
// Version:
//   - 2026-09-22: Added.
func (f *fakeDependencies) Available(context.Context) ([]string, error) { return f.versions, nil }

// Compile records the selected version without accessing a runtime or network.
//
// Version:
//   - 2026-09-22: Added.
func (f *fakeDependencies) Compile(_ context.Context, request CompileRequest) (json.RawMessage, error) {
	f.compiled = append(f.compiled, request.Version)
	if f.compile != nil {
		return f.compile(request)
	}
	return f.fixture.Output, f.compileErr
}

func loadFixture(t *testing.T, name string) fixture {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var result fixture
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func analyzeFixture(t *testing.T, f *fakeDependencies) (Result, error) {
	t.Helper()
	a, err := NewAnalyzer(f, f, f)
	if err != nil {
		t.Fatal(err)
	}
	return a.Analyze(context.Background(), Request{ChainID: evm.ChainIDBaseMainnet, Token: common.HexToAddress("0x1234"), BlockHash: common.HexToHash("0x5678")})
}

// TestAnalyzeSourceModels verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestAnalyzeSourceModels(t *testing.T) {
	for _, tt := range []struct {
		name, model string
		available   bool
	}{
		{"simple", "openzeppelin-erc20-v5.5-v1", true},
		{"permit", "openzeppelin-erc20-permit-v5.5-v1", true},
		{"taxed", "", false},
		{"getter", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeDependencies{fixture: loadFixture(t, tt.name), versions: []string{"0.8.37+commit.f401782d"}}
			original := append([]byte(nil), f.fixture.Bundle.Input...)
			result, err := analyzeFixture(t, f)
			if err != nil {
				t.Fatal(err)
			}
			if (result.Observation != nil) != tt.available || result.Model != tt.model {
				t.Fatalf("unexpected result: %+v", result)
			}
			if tt.available {
				o := result.Observation
				if o.BuyRate == nil || *o.BuyRate != "0" || o.SellRate == nil || *o.SellRate != "0" || o.CanChange == nil || *o.CanChange || o.HasExemptions == nil || *o.HasExemptions {
					t.Fatalf("unexpected tax observation: %+v", o)
				}
				if result.UsedCompiler != "0.8.37+commit.f401782d" || result.CodeSHA256 == "" {
					t.Fatalf("missing evidence: %+v", result)
				}
			} else if result.Reason != "unsupported_model" {
				t.Fatalf("reason = %q", result.Reason)
			}
			if len(f.compiled) != 1 || f.reads != 1 || f.sourceCalls != 1 {
				t.Fatalf("unexpected I/O: %+v", f.compiled)
			}
			if f.hash != result.BlockHash || !reflect.DeepEqual(original, []byte(f.fixture.Bundle.Input)) {
				t.Fatal("observation position or input changed")
			}
		})
	}
}

// TestAnalyzeDefinitionsAndReviewedCode verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestAnalyzeDefinitionsAndReviewedCode(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		token                common.Address
		file, reason, source string
	}{
		{name: "native", source: "native_currency"},
		{name: "trusted usdc", token: common.HexToAddress("0x833589fcd6edb6e08f4c7c32d4f71b54bda02913"), reason: "trusted_token_skipped"},
		{name: "weth runtime", token: common.HexToAddress("0x1234"), file: "weth9-runtime", source: "contract_analysis"},
		{name: "taot runtime", token: common.HexToAddress("0x1234"), file: "taot-runtime", source: "contract_analysis"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeDependencies{}
			if tt.file != "" {
				f.fixture = loadFixture(t, tt.file)
			}
			a, err := NewAnalyzer(f, f, f)
			if err != nil {
				t.Fatal(err)
			}
			r, err := a.Analyze(context.Background(), Request{ChainID: evm.ChainIDBaseMainnet, Token: tt.token, BlockHash: common.HexToHash("0x1")})
			if err != nil {
				t.Fatal(err)
			}
			if r.Reason != tt.reason || (r.Observation == nil) != (tt.source == "") {
				t.Fatalf("unexpected result: %+v", r)
			}
			if r.Observation != nil && r.Observation.Source != tt.source {
				t.Fatalf("source = %q", r.Observation.Source)
			}
			wantReads := 0
			if tt.file != "" {
				wantReads = 1
			}
			if f.reads != wantReads || f.sourceCalls != 0 || len(f.compiled) != 0 {
				t.Fatal("unexpected acquisition")
			}
		})
	}
}

// TestAnalyzeRejectsUnverifiedInputs verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestAnalyzeRejectsUnverifiedInputs(t *testing.T) {
	for _, name := range []string{"runtime mismatch", "changed dependency", "missing dependency", "source url", "unknown proxy", "bad metadata", "unexpected immutable"} {
		t.Run(name, func(t *testing.T) {
			f := &fakeDependencies{fixture: loadFixture(t, "simple"), versions: []string{"0.8.37+commit.f401782d"}}
			var input map[string]any
			if err := json.Unmarshal(f.fixture.Bundle.Input, &input); err != nil {
				t.Fatal(err)
			}
			sources := input["sources"].(map[string]any)
			switch name {
			case "runtime mismatch":
				f.fixture.Code = "0x00" + f.fixture.Code[4:]
			case "unknown proxy":
				f.fixture.Code = "0x363d3d373d3d3d363d73" + strings.Repeat("11", 20) + "5af43d82803e903d91602b57fd5bf3"
			case "changed dependency":
				sources["@openzeppelin/contracts/token/ERC20/ERC20.sol"].(map[string]any)["content"] = "contract ERC20 {}"
			case "missing dependency":
				delete(sources, "@openzeppelin/contracts/token/ERC20/ERC20.sol")
			case "source url":
				sources["FixtureToken.sol"].(map[string]any)["urls"] = []string{"/etc/passwd"}
			case "bad metadata":
				f.fixture.Code = f.fixture.Code[:len(f.fixture.Code)-4] + "ffff"
			case "unexpected immutable":
				var output map[string]any
				if err := json.Unmarshal(f.fixture.Output, &output); err != nil {
					t.Fatal(err)
				}
				artifact := output["contracts"].(map[string]any)["FixtureToken.sol"].(map[string]any)["FixtureToken"].(map[string]any)["evm"].(map[string]any)["deployedBytecode"].(map[string]any)
				artifact["immutableReferences"] = map[string]any{"999999": []any{map[string]any{"start": 1, "length": 32}}}
				var err error
				f.fixture.Output, err = json.Marshal(output)
				if err != nil {
					t.Fatal(err)
				}
			}
			var err error
			f.fixture.Bundle.Input, err = json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			r, err := analyzeFixture(t, f)
			if err != nil {
				t.Fatal(err)
			}
			if r.Observation != nil || r.Reason == "" {
				t.Fatalf("unverified token accepted: %+v", r)
			}
		})
	}
}

// TestAnalyzeCompilerFallbackAndErrors verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestAnalyzeCompilerFallbackAndErrors(t *testing.T) {
	t.Run("bundled version succeeds without original download", func(t *testing.T) {
		f := &fakeDependencies{fixture: loadFixture(t, "simple"), versions: []string{"0.8.37+commit.f401782d"}}
		f.fixture.Bundle.CompilerVersion = "0.8.34+commit.80d5c536"
		f.fixture.Code = loadFixture(t, "simple-0834-runtime").Code
		r, err := analyzeFixture(t, f)
		if err != nil {
			t.Fatal(err)
		}
		if r.Observation == nil || !reflect.DeepEqual(f.compiled, []string{"0.8.37+commit.f401782d"}) || r.OriginalCompiler == r.UsedCompiler {
			t.Fatalf("fallback was not avoided: %+v %v", r, f.compiled)
		}
	})
	t.Run("compiler failure then original", func(t *testing.T) {
		f := &fakeDependencies{fixture: loadFixture(t, "simple"), versions: []string{"0.8.34+commit.80d5c536"}}
		f.compile = func(req CompileRequest) (json.RawMessage, error) {
			if req.Version == "0.8.34+commit.80d5c536" {
				return json.RawMessage(`{"errors":[{"severity":"error"}]}`), nil
			}
			return f.fixture.Output, nil
		}
		r, err := analyzeFixture(t, f)
		if err != nil {
			t.Fatal(err)
		}
		if r.Observation == nil || len(f.compiled) != 2 {
			t.Fatalf("missing original fallback: %+v %v", r, f.compiled)
		}
	})
	for _, stage := range []string{"rpc", "source", "compiler"} {
		t.Run(stage, func(t *testing.T) {
			failure := errors.New("acquisition failed")
			f := &fakeDependencies{fixture: loadFixture(t, "simple")}
			switch stage {
			case "rpc":
				f.readErr = failure
			case "source":
				f.sourceErr = failure
			case "compiler":
				f.compileErr = failure
			}
			r, err := analyzeFixture(t, f)
			if !errors.Is(err, failure) || r.Observation != nil {
				t.Fatalf("failure chain lost: %+v %v", r, err)
			}
		})
	}
	t.Run("unsupported compiler stays unknown", func(t *testing.T) {
		f := &fakeDependencies{fixture: loadFixture(t, "simple"), compileErr: ErrUnsupported}
		r, err := analyzeFixture(t, f)
		if err != nil || r.Observation != nil {
			t.Fatalf("unexpected result: %+v %v", r, err)
		}
	})
	t.Run("malformed compiler output", func(t *testing.T) {
		f := &fakeDependencies{fixture: loadFixture(t, "simple")}
		f.fixture.Output = json.RawMessage(`{`)
		r, err := analyzeFixture(t, f)
		if err == nil || r.Observation != nil {
			t.Fatal("invalid output accepted")
		}
	})
}

// TestAnalyzerCompositionAndCancellation verifies the named analysis or transport invariant.
//
// Version:
//   - 2026-09-22: Added.
func TestAnalyzerCompositionAndCancellation(t *testing.T) {
	f := &fakeDependencies{}
	for _, deps := range []struct {
		r Reader
		s SourceProvider
		c Compiler
	}{{nil, f, f}, {f, nil, f}, {f, f, nil}} {
		if _, err := NewAnalyzer(deps.r, deps.s, deps.c); err == nil {
			t.Fatal("missing dependency accepted")
		}
	}
	a, err := NewAnalyzer(f, f, f)
	if err != nil {
		t.Fatal(err)
	}
	if a.reader != f || a.sources != f || a.compiler != f {
		t.Fatal("dependencies not composed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = a.Analyze(ctx, Request{})
	if !errors.Is(err, context.Canceled) || f.reads != 0 {
		t.Fatal("canceled analysis performed I/O")
	}
	r, err := a.Analyze(context.Background(), Request{ChainID: evm.ChainIDBaseSepolia, BlockHash: common.HexToHash("0x1")})
	if err != nil || r.Reason != "unsupported_chain" || f.reads != 0 {
		t.Fatal("unsupported chain performed I/O")
	}
}
