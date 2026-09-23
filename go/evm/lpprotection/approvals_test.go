package lpprotection

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

func approvalEvent(manager, owner, operator common.Address, approved int64) types.Log {
	return types.Log{Address: manager, Topics: []common.Hash{crypto.Keccak256Hash([]byte("ApprovalForAll(address,address,bool)")), common.BytesToHash(owner[:]), common.BytesToHash(operator[:])}, Data: words(big.NewInt(approved)), BlockNumber: 1, BlockHash: common.HexToHash("0x31"), TxHash: common.HexToHash("0x32")}
}

func vaultFixture(t *testing.T) (*fakeRPC, Request, common.Address, common.Address) {
	t.Helper()
	f, req, manager := fixture(t, clliquidity.V3)
	owner := common.HexToAddress("0x300001")
	f.codes[owner] = runtime(t, "vault")
	f.integer(owner, "vaultKeyId()", new(big.Int))
	f.integer(owner, "unlockTimestamp()", big.NewInt(1800000100))
	f.integer(owner, "isUnlocked()", new(big.Int))
	return f, req, manager, owner
}

// TestOperatorProof requires full history and current operator state for a lock verdict.
//
// Version:
//   - 2026-09-23: Added.
func TestOperatorProof(t *testing.T) {
	operator := common.HexToAddress("0x700001")
	for _, tc := range []struct {
		name          string
		current       int64
		eventValue    int64
		wantAvailable bool
	}{
		{"active before pool creation", 1, 1, false},
		{"revoked operator", 0, 1, true},
		{"use current state instead of last event", 1, 0, false},
		{"invalid boolean", 2, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, req, manager, owner := vaultFixture(t)
			f.approvalLogs = []types.Log{approvalEvent(manager, owner, operator, tc.eventValue)}
			f.integer(manager, "isApprovedForAll(address,address)", big.NewInt(tc.current), owner.Big(), operator.Big())
			result, err := run(t, f, req)
			if tc.current > 1 {
				if err == nil || result.Observation != nil {
					t.Fatalf("%+v %v", result, err)
				}
				return
			}
			if err != nil || (result.Observation != nil) != tc.wantAvailable {
				t.Fatalf("%+v %v", result, err)
			}
			if !tc.wantAvailable && (result.Positions[0].Kind != "" || result.Positions[0].CanWeakenProtection != nil || result.Positions[0].Reason != "operator_approval_active") {
				t.Fatalf("unsafe position: %+v", result.Positions[0])
			}
			if len(f.queries) != 1 || f.queries[0].FromBlock.Sign() != 0 || f.queries[0].ToBlock.Uint64() != req.Principal.Snapshot.BlockNumber {
				t.Fatal("operator history was not complete")
			}
		})
	}
	t.Run("retrieval failure", func(t *testing.T) {
		f, req, _, _ := vaultFixture(t)
		sentinel := errors.New("history unavailable")
		f.approvalErr = sentinel
		result, err := run(t, f, req)
		if !errors.Is(err, sentinel) || result.Observation != nil || result.Positions[0].Kind != "" {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("budget preflight", func(t *testing.T) {
		f, req, _, _ := vaultFixture(t)
		lim := limits()
		lim.LogBlockRange = 1
		lim.MaxCalls = 32
		r, err := NewReader(f, lim)
		if err != nil {
			t.Fatal(err)
		}
		result, err := r.Analyze(context.Background(), req)
		if !errors.Is(err, ErrBudget) || result.Observation != nil || len(f.queries) != 0 || result.Positions[0].Reason != "operator_history_unresolved" {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("empty ranges through observation", func(t *testing.T) {
		f, req, _, _ := vaultFixture(t)
		lim := limits()
		lim.LogBlockRange = 40
		r, err := NewReader(f, lim)
		if err != nil {
			t.Fatal(err)
		}
		result, err := r.Analyze(context.Background(), req)
		if err != nil || result.Observation == nil || len(f.queries) != 3 {
			t.Fatalf("%+v %v", result, err)
		}
		for i, expected := range [][2]uint64{{0, 39}, {40, 79}, {80, 100}} {
			if f.queries[i].FromBlock.Uint64() != expected[0] || f.queries[i].ToBlock.Uint64() != expected[1] {
				t.Fatal("approval history gap")
			}
		}
	})
	for _, kind := range []string{"removed", "wrong owner", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			f, req, manager, owner := vaultFixture(t)
			event := approvalEvent(manager, owner, operator, 1)
			switch kind {
			case "removed":
				event.Removed = true
			case "wrong owner":
				event.Topics[1] = common.Hash{}
			case "malformed":
				event.Data = []byte{1}
			}
			f.approvalLogs = []types.Log{event}
			result, err := run(t, f, req)
			if err == nil || result.Observation != nil {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}
