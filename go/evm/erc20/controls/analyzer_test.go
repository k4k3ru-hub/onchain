package controls

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
	"github.com/k4k3ru-hub/onchain/go/evm/erc20/analysis"
)

type fixtureRPC struct {
	Bundle                          analysis.SourceBundle `json:"bundle"`
	Code                            string                `json:"code"`
	Output                          json.RawMessage       `json:"output"`
	owner                           common.Address
	stateErr                        error
	reads, sources, compiles, calls int
	hashes                          []common.Hash
}

// CodeAtHash serves saved runtime at the requested hash.
//
// Version:
//   - 2026-09-23: Added.
func (f *fixtureRPC) CodeAtHash(_ context.Context, _ common.Address, h common.Hash) ([]byte, error) {
	f.reads++
	f.hashes = append(f.hashes, h)
	return hex.DecodeString(strings.TrimPrefix(f.Code, "0x"))
}

// Source supplies the source used to compile the independent fixture.
//
// Version:
//   - 2026-09-23: Added.
func (f *fixtureRPC) Source(context.Context, analysis.SourceRequest) (analysis.SourceBundle, error) {
	f.sources++
	return f.Bundle, nil
}

// Available supplies the native compiler version used for the fixture.
//
// Version:
//   - 2026-09-23: Added.
func (f *fixtureRPC) Available(context.Context) ([]string, error) {
	return []string{"0.8.37+commit.f401782d"}, nil
}

// Compile supplies actual native output without executing a compiler in unit tests.
//
// Version:
//   - 2026-09-23: Added.
func (f *fixtureRPC) Compile(context.Context, analysis.CompileRequest) (json.RawMessage, error) {
	f.compiles++
	return f.Output, nil
}

// CallContractAtHash records the owner read and returns a configured state.
//
// Version:
//   - 2026-09-23: Added.
func (f *fixtureRPC) CallContractAtHash(_ context.Context, m ethereum.CallMsg, h common.Hash) ([]byte, error) {
	f.calls++
	f.hashes = append(f.hashes, h)
	if hex.EncodeToString(m.Data) != "8da5cb5b" {
		return nil, errors.New("unexpected selector")
	}
	return common.LeftPadBytes(f.owner.Bytes(), 32), f.stateErr
}

func load(t *testing.T, name string) *fixtureRPC {
	t.Helper()
	folder := "../tax/testdata/"
	if strings.HasPrefix(name, "ownable") || name == "oni-agent" {
		folder = "../analysis/testdata/"
	}
	raw, err := os.ReadFile(folder + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixtureRPC{}
	if err = json.Unmarshal(raw, f); err != nil {
		t.Fatal(err)
	}
	return f
}
func compose(t *testing.T, f *fixtureRPC) *Analyzer {
	t.Helper()
	code, err := analysis.NewAnalyzer(f, f, f)
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAnalyzer(code, f)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestReviewedControls verifies actual compiled structures and fixed-block authority.
//
// Version:
//   - 2026-09-23: Added.
func TestReviewedControls(t *testing.T) {
	for _, name := range []string{"simple", "permit", "ownable", "ownable-mint", "weth9-runtime", "taot-runtime", "taxed", "getter", "ownable-recovery", "ownable-unrestricted-mint"} {
		t.Run(name, func(t *testing.T) {
			f := load(t, name)
			f.owner = common.HexToAddress("0x1234")
			a := compose(t, f)
			hash := common.HexToHash("0x5678")
			r, err := a.Analyze(t.Context(), Request{ChainID: evm.ChainIDBaseMainnet, Token: common.HexToAddress("0x9999"), BlockHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			supported := name == "simple" || name == "permit" || name == "ownable" || name == "ownable-mint" || name == "weth9-runtime"
			if (r.Observation.Upgrade.CanUpgrade.Status == "observed") != supported {
				t.Fatalf("unexpected recognition: %s %+v", r.Evidence.Model, r.Observation)
			}
			if !supported {
				if r.Observation.Minting.CanMint.Value != nil {
					t.Fatal("unsupported became false")
				}
				return
			}
			o := r.Observation
			if *o.Upgrade.CanUpgrade.Value || *o.BalanceControl.CanForceTransfer.Value || *o.TransferRestrictions.BlacklistPresent.Value {
				t.Fatal("unexpected capability")
			}
			wantsOwner := strings.HasPrefix(name, "ownable")
			if wantsOwner {
				if f.calls != 1 || *o.Ownership.Renounced.Value {
					t.Fatal("owner observation missing")
				}
			} else if o.Ownership.Renounced.Status != "not_applicable" || f.calls != 0 {
				t.Fatal("no owner confused with renunciation")
			}
			if *o.Minting.CanMint.Value != (name == "ownable-mint" || name == "weth9-runtime") {
				t.Fatal("wrong mint permission")
			}
			for _, h := range f.hashes {
				if h != hash {
					t.Fatal("mixed observation blocks")
				}
			}
		})
	}
}

// TestOwnerStateAndPolicy verifies renunciation, partial errors and trusted skips.
//
// Version:
//   - 2026-09-23: Added.
func TestOwnerStateAndPolicy(t *testing.T) {
	f := load(t, "ownable-mint")
	a := compose(t, f)
	request := Request{ChainID: evm.ChainIDBaseMainnet, Token: common.HexToAddress("0x9999"), BlockHash: common.HexToHash("0x12")}
	r, err := a.Analyze(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !*r.Observation.Minting.Present.Value || *r.Observation.Minting.CanMint.Value || !*r.Observation.Ownership.Renounced.Value {
		t.Fatal("renounced mint not disabled")
	}
	sentinel := errors.New("state unavailable")
	f.stateErr = sentinel
	r, err = a.Analyze(t.Context(), request)
	if !errors.Is(err, sentinel) || r.Observation.Minting.CanMint.Value != nil || r.Observation.Ownership.Renounced.Value != nil || r.Observation.Upgrade.CanUpgrade.Value == nil {
		t.Fatal("partial state failure corrupted facts")
	}
	for _, address := range []string{"0x0000000000000000000000000000000000000000", "0x4200000000000000000000000000000000000006", "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"} {
		f := &fixtureRPC{}
		a := compose(t, f)
		r, err := a.Analyze(t.Context(), Request{ChainID: evm.ChainIDBaseMainnet, Token: common.HexToAddress(address)})
		if err != nil {
			t.Fatal(err)
		}
		if r.Policy == "" || f.reads+f.sources+f.compiles+f.calls != 0 || r.Observation.Minting.CanMint.Value != nil {
			t.Fatal("policy fabricated observation or performed I/O")
		}
	}
	if _, err := NewAnalyzer(nil, f); err == nil {
		t.Fatal("missing dependency accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := a.Analyze(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}
