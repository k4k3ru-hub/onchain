package controls

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

// TestOniAgentControls verifies exact runtime recognition and per-token owner state.
//
// Version:
//   - 2026-09-23: Added.
func TestOniAgentControls(t *testing.T) {
	f := load(t, "oni-agent")
	a := compose(t, f)
	r := Request{ChainID: evm.ChainIDBaseMainnet, Token: common.HexToAddress("0x66397356a1526f42ef1709de9d3ecbaae76a653b"), BlockHash: common.HexToHash("0x1234")}
	for _, owner := range []common.Address{common.HexToAddress("0x1111"), {}} {
		f.owner = owner
		f.hashes = nil
		result, err := a.Analyze(t.Context(), r)
		if err != nil {
			t.Fatal(err)
		}
		o := result.Observation
		if result.Evidence.Model != "oni-agent-reviewed-runtime-v1" || o.Ownership.OwnerAddress.Value == nil || *o.Ownership.OwnerAddress.Value != owner.Hex() || *o.Ownership.Renounced.Value != (owner == (common.Address{})) {
			t.Fatal("incorrect owner observation", o.Ownership)
		}
		for _, flag := range []BoolFinding{o.TransferRestrictions.BlacklistPresent, o.TransferRestrictions.AllowlistPresent, o.TransferRestrictions.AutomaticBuyerRestrictionPresent, o.TransferRestrictions.TransferLimitsPresent, o.TransferRestrictions.PausePresent, o.TransferRestrictions.Paused, o.TransferRestrictions.CanChange, o.Minting.Present, o.Minting.CanMint, o.Upgrade.CanUpgrade, o.BalanceControl.CanForceTransfer, o.BalanceControl.CanForceBurn} {
			if flag.Status != "observed" || flag.Value == nil || *flag.Value {
				t.Fatal("metadata ownership inferred trading privileges", flag)
			}
		}
		if o.Minting.Authorization.Status != "not_applicable" || o.Minting.SupplyCapRaw.Value != nil {
			t.Fatal("constructor issuance confused with runtime mint")
		}
		for _, hash := range f.hashes {
			if hash != r.BlockHash {
				t.Fatal("observation blocks mixed")
			}
		}
		r.Token = common.HexToAddress("0xc82d083dc42b302d90239a30ce6b32c87255205d")
		r.BlockHash = common.HexToHash("0x5678")
	}
	f.stateErr = errors.New("owner unavailable")
	result, err := a.Analyze(t.Context(), r)
	if !errors.Is(err, f.stateErr) || result.Observation.Ownership.Renounced.Value != nil || result.Observation.Ownership.Renounced.Status != "unknown" || result.Observation.Minting.CanMint.Value == nil || *result.Observation.Minting.CanMint.Value {
		t.Fatal("owner error lost independent code facts")
	}
	if f.calls != 3 || f.reads != 3 || f.sources != 0 || f.compiles != 0 {
		t.Fatalf("unexpected acquisition: calls=%d reads=%d sources=%d compiles=%d", f.calls, f.reads, f.sources, f.compiles)
	}
}

// TestOniAgentRejectsRuntimeVariants rejects changed executable and metadata bytes.
//
// Version:
//   - 2026-09-23: Added.
func TestOniAgentRejectsRuntimeVariants(t *testing.T) {
	for _, name := range []string{"executable", "metadata"} {
		t.Run(name, func(t *testing.T) {
			f := load(t, "oni-agent")
			code, err := hex.DecodeString(strings.TrimPrefix(f.Code, "0x"))
			if err != nil {
				t.Fatal(err)
			}
			at := 0
			if name == "metadata" {
				at = len(code) - 20
			}
			code[at] ^= 1
			f.Code = hex.EncodeToString(code)
			r, err := compose(t, f).Analyze(t.Context(), Request{ChainID: evm.ChainIDBaseMainnet, Token: common.HexToAddress("0x9999"), BlockHash: common.HexToHash("0x1234")})
			if err != nil {
				t.Fatal(err)
			}
			if r.Evidence.Model != "" || r.Observation.Minting.CanMint.Value != nil || f.calls != 0 {
				t.Fatal("unreviewed runtime acquired trusted facts")
			}
		})
	}
}
