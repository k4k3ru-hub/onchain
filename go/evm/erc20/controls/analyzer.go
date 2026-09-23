package controls

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
	"github.com/k4k3ru-hub/onchain/go/evm/erc20/analysis"
)

const ModelVersion = "evm-token-controls-v1"

type CodeAnalyzer interface {
	Analyze(context.Context, analysis.Request) (analysis.Result, error)
}
type StateReader interface {
	CallContractAtHash(context.Context, ethereum.CallMsg, common.Hash) ([]byte, error)
}
type Request struct {
	ChainID   evm.ChainID
	Token     common.Address
	BlockHash common.Hash
}
type Result struct {
	Observation *Observation
	Evidence    analysis.Result
	Policy      string
}
type Analyzer struct {
	code  CodeAnalyzer
	state StateReader
}

// NewAnalyzer composes shared code verification and a fixed-hash state reader.
//
// Version:
//   - 2026-09-23: Added.
func NewAnalyzer(code CodeAnalyzer, state StateReader) (*Analyzer, error) {
	if code == nil || state == nil {
		return nil, fmt.Errorf("failed to create token controls analyzer: dependency=null")
	}
	return &Analyzer{code: code, state: state}, nil
}

// Analyze evaluates supported code and state without interpreting unknown as false.
// Defined tokens skip acquisition. A state error retains independently verified facts.
// The caller owns latest-block selection, canonicality, retries and timestamps.
//
// Version:
//   - 2026-09-23: Added.
func (a *Analyzer) Analyze(ctx context.Context, r Request) (Result, error) {
	out := Result{Observation: unknown("unknown", "unsupported_model")}
	if a == nil || a.code == nil || a.state == nil {
		return out, fmt.Errorf("failed to analyze token controls: analyzer=null")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return out, fmt.Errorf("failed to analyze token controls: %w", err)
	}
	if r.ChainID != evm.ChainIDBaseMainnet {
		out.Observation = unknown("unknown", "unsupported_chain")
		return out, nil
	}
	if _, defined := evm.LookupTokenMetadata(r.ChainID, r.Token); defined {
		out.Policy = "sdk_definition"
		out.Observation = unknown("trusted", out.Policy)
		if r.Token == (common.Address{}) {
			out.Policy = "native_currency"
			out.Observation = unknown("not_applicable", out.Policy)
		}
		return out, nil
	}
	if r.BlockHash == (common.Hash{}) {
		return out, fmt.Errorf("failed to analyze token controls: block_hash=empty")
	}
	evidence, err := a.code.Analyze(ctx, analysis.Request(r))
	out.Evidence = evidence
	if err != nil {
		out.Observation = unknown("unknown", "source_unavailable")
		return out, fmt.Errorf("failed to analyze token controls: %w", err)
	}
	own, mint, wrapped := false, false, false
	switch evidence.Model {
	case "openzeppelin-erc20-v5.5-v1", "openzeppelin-erc20-permit-v5.5-v1":
	case "openzeppelin-erc20-ownable-v5.5-v1":
		own = true
	case "openzeppelin-erc20-ownable-mint-v5.5-v1":
		own = true
		mint = true
	case "weth9-reviewed-runtime-v1":
		wrapped = true
	default:
		return out, nil
	}
	o := unknown("unknown", "unsupported_model")
	out.Observation = o
	o.Ownership = Ownership{OwnerAddress: StringFinding{Status: "not_applicable", Reason: "no_ownership_mechanism"}, Renounced: BoolFinding{Status: "not_applicable", Reason: "no_ownership_mechanism"}}
	o.TransferRestrictions = TransferRestrictions{BlacklistPresent: boolean(false), AllowlistPresent: boolean(false), AutomaticBuyerRestrictionPresent: boolean(false), TransferLimitsPresent: boolean(false), PausePresent: boolean(false), Paused: boolean(false), CanChange: boolean(false)}
	o.Upgrade.CanUpgrade = boolean(false)
	o.BalanceControl = BalanceControl{CanForceTransfer: boolean(false), CanForceBurn: boolean(false)}
	o.Minting = Minting{Present: boolean(mint || wrapped), CanMint: boolean(wrapped), Authorization: StringFinding{Status: "not_applicable", Reason: "no_runtime_mint"}, RequiresBacking: BoolFinding{Status: "not_applicable", Reason: "no_runtime_mint"}, HasSupplyCap: BoolFinding{Status: "not_applicable", Reason: "no_runtime_mint"}, SupplyCapRaw: StringFinding{Status: "not_applicable", Reason: "no_runtime_mint"}}
	if mint || wrapped {
		o.Minting.RequiresBacking = boolean(wrapped)
		o.Minting.HasSupplyCap = boolean(false)
		o.Minting.SupplyCapRaw = StringFinding{Status: "not_applicable", Reason: "no_supply_cap"}
		o.Minting.Authorization = text("permissionless")
		if mint {
			o.Minting.Authorization = text("owner")
			o.Minting.CanMint = BoolFinding{Status: "unknown", Reason: "state_unavailable"}
		}
	}
	if !own {
		return out, nil
	}
	o.Ownership = Ownership{OwnerAddress: StringFinding{Status: "unknown", Reason: "state_unavailable"}, Renounced: BoolFinding{Status: "unknown", Reason: "state_unavailable"}}
	raw, err := a.state.CallContractAtHash(ctx, ethereum.CallMsg{To: &r.Token, Data: []byte{0x8d, 0xa5, 0xcb, 0x5b}, Gas: 100_000}, r.BlockHash)
	if err != nil {
		return out, fmt.Errorf("failed to read token owner: %w", err)
	}
	if len(raw) != 32 || !bytes.Equal(raw[:12], make([]byte, 12)) {
		return out, fmt.Errorf("failed to read token owner: response=invalid")
	}
	owner := common.BytesToAddress(raw[12:])
	renounced := owner == (common.Address{})
	o.Ownership = Ownership{OwnerAddress: text(owner.Hex()), Renounced: boolean(renounced)}
	if mint {
		o.Minting.CanMint = boolean(!renounced)
	}
	return out, nil
}

func boolean(v bool) BoolFinding  { return BoolFinding{Status: "observed", Value: &v} }
func text(v string) StringFinding { return StringFinding{Status: "observed", Value: &v} }
