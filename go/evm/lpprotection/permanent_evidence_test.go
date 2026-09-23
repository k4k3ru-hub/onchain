package lpprotection

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// TestPermanentEvidenceMigration preserves v4 failure ledgers instead of restarting abandoned history.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentEvidenceMigration(t *testing.T) {
	f, req := creationSample(t)
	f.failFrom = 51592853
	for attempt := 1; attempt <= 4; attempt++ {
		result, err := analyzeCreation(t, f, req)
		if err == nil || result.Evidence == nil {
			t.Fatalf("%+v %v", result, err)
		}
		req.Evidence = result.Evidence
	}
	original := req.Evidence
	b, err := original.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	old := []byte(strings.Replace(string(b), ModelVersion, previousModelVersion, 1))
	restored, err := RestoreEvidence(old, 512<<10)
	if err != nil || restored.data.ModelVersion != ModelVersion || len(restored.PermanentCustodies()) != 0 || !reflect.DeepEqual(restored.data.Histories, original.data.Histories) || !reflect.DeepEqual(restored.data.Creations, original.data.Creations) || !reflect.DeepEqual(restored.data.Acquired, original.data.Acquired) {
		t.Fatalf("migration changed old evidence: %v", err)
	}
	req.Evidence = restored
	f.queries = nil
	f.failFrom = 0
	result, err := analyzeCreation(t, f, req)
	if !errors.Is(err, ErrHistoryAbandoned) || len(f.queries) != 0 || result.Observation != nil || result.Evidence.Histories()[0].Failure.Attempts != 4 {
		t.Fatalf("abandoned acquisition restarted: %+v %v", result, err)
	}
	var invalid map[string]any
	if err := json.Unmarshal(old, &invalid); err != nil {
		t.Fatal(err)
	}
	invalid["PermanentCustodies"] = nil
	bad, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreEvidence(bad, 512<<10); err == nil {
		t.Fatal("old schema accepted new proof field")
	}
	delete(invalid, "PermanentCustodies")
	invalid["Histories"].([]any)[0].(map[string]any)["Failure"].(map[string]any)["Attempts"] = 5
	bad, err = json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreEvidence(bad, 512<<10); err == nil {
		t.Fatal("old validator bypassed")
	}
	if _, err := RestoreEvidence(old, len(old)-1); !errors.Is(err, ErrBudget) {
		t.Fatalf("size limit bypassed: %v", err)
	}
}

// TestPermanentEvidenceValidation rejects fabricated proof bindings and detached-copy mutations.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentEvidenceValidation(t *testing.T) {
	f, req := permanentSample(t)
	result, err := permanentRun(t, f, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := result.Evidence.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"nonce", "sender", "rule", "duplicate", "conflict", "version"} {
		t.Run(mode, func(t *testing.T) {
			var d evidenceData
			if err := json.Unmarshal(b, &d); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "nonce":
				d.PermanentCustodies[0].LockerNonce = 2
			case "sender":
				d.PermanentCustodies[0].Sender = common.HexToAddress("0x1")
			case "rule":
				d.PermanentCustodies[0].Rule = "unreviewed"
			case "duplicate":
				d.PermanentCustodies = append(d.PermanentCustodies, d.PermanentCustodies[0])
			case "conflict":
				d.PermanentConflicts = []permanentConflict{{}}
			case "version":
				d.ModelVersion = previousModelVersion
			}
			bad, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := RestoreEvidence(bad, 512<<10); err == nil {
				t.Fatal("invalid proof accepted")
			}
		})
	}
}

// TestPermanentAuthorityEvents records confirmed contradictions supplied as receipts without scanning history.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentAuthorityEvents(t *testing.T) {
	for _, mode := range []string{"operator", "approval", "transfer", "different owner", "different manager", "revocation", "removed"} {
		t.Run(mode, func(t *testing.T) {
			f, req := permanentSample(t)
			manager := common.HexToAddress("0x7c5f5a4bbd8fd63184577525326123b519429bdc")
			locker := crypto.CreateAddress(req.CreationHints[0].Factory, 1)
			other := common.HexToAddress("0x123456")
			l := types.Log{Address: manager, Topics: []common.Hash{crypto.Keccak256Hash([]byte("ApprovalForAll(address,address,bool)")), common.BytesToHash(locker[:]), common.BytesToHash(other[:])}, Data: words(big.NewInt(1)), BlockNumber: req.Principal.Snapshot.BlockNumber, BlockHash: req.Principal.Snapshot.BlockHash, TxHash: common.HexToHash("0x123456"), Index: 1}
			switch mode {
			case "approval":
				l.Topics[0] = crypto.Keccak256Hash([]byte("Approval(address,address,uint256)"))
				l.Topics = append(l.Topics, common.BigToHash(big.NewInt(3073314)))
				l.Data = nil
			case "transfer":
				l.Topics[0] = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
				l.Topics = append(l.Topics, common.BigToHash(big.NewInt(3073314)))
				l.Data = nil
			case "different owner":
				l.Topics[1] = common.BytesToHash(other[:])
			case "different manager":
				l.Address = other
			case "revocation":
				l.Data = words(new(big.Int))
			case "removed":
				l.Removed = true
			}
			req.Receipts = map[common.Hash]*types.Receipt{l.TxHash: {Status: 1, TxHash: l.TxHash, BlockNumber: new(big.Int).SetUint64(l.BlockNumber), BlockHash: l.BlockHash, Logs: []*types.Log{&l}}}
			result, err := permanentRun(t, f, req)
			wantProtected := mode == "different owner" || mode == "different manager" || mode == "revocation"
			if (result.Observation != nil) != wantProtected || len(f.queries) != 0 {
				t.Fatalf("%+v %v", result, err)
			}
			if mode == "removed" {
				if err == nil {
					t.Fatal("removed proof accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !wantProtected {
				if len(result.Evidence.data.PermanentConflicts) != 1 {
					t.Fatal("contradiction not retained")
				}
				req.Evidence = result.Evidence
				req.Receipts = nil
				result, err = permanentRun(t, f, req)
				if err != nil || result.Observation != nil {
					t.Fatalf("restored conflict lost: %+v %v", result, err)
				}
			}
		})
	}
}

// TestPermanentConflictReorg removes only an orphaned conflict and preserves unrelated canonical conflicts.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentConflictReorg(t *testing.T) {
	f, req := permanentSample(t)
	result, err := permanentRun(t, f, req)
	if err != nil {
		t.Fatal(err)
	}
	proof := result.Evidence.PermanentCustodies()[0]
	orphan := &types.Header{Number: big.NewInt(51604300), Time: 1789997947}
	canonical := types.CopyHeader(orphan)
	canonical.Extra = []byte("replacement")
	f.data.Headers[51604300] = canonical
	first := permanentConflict{Manager: proof.Manager, Custodian: proof.Custodian, Block: BlockReference{Number: 51604300, Hash: orphan.Hash()}}
	second := permanentConflict{Manager: proof.Manager, Custodian: proof.Custodian, Block: BlockReference{Number: req.Creation.BlockNumber, Hash: req.Creation.BlockHash}}
	result.Evidence.data.PermanentConflicts = []permanentConflict{first, second}
	req.Evidence = result.Evidence
	result, err = permanentRun(t, f, req)
	if !errors.Is(err, ErrReorg) || result.Observation != nil || len(result.Evidence.PermanentCustodies()) != 0 || len(result.Evidence.data.PermanentConflicts) != 1 || result.Evidence.data.PermanentConflicts[0] != second {
		t.Fatalf("wrong conflict invalidation: %+v %v", result, err)
	}
	req.Evidence = result.Evidence
	result, err = permanentRun(t, f, req)
	if err != nil || result.Observation != nil || result.Positions[0].Reason != "authority_conflict" {
		t.Fatalf("canonical conflict lost: %+v %v", result, err)
	}
}

// TestPermanentResolver composes automatic creation lookup while retaining candidate verification.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentResolver(t *testing.T) {
	for _, mode := range []string{"valid", "wrong factory", "missing", "error"} {
		t.Run(mode, func(t *testing.T) {
			f, req := permanentSample(t)
			hint := req.CreationHints[0]
			req.CreationHints = nil
			calls := 0
			sentinel := errors.New("lookup unavailable")
			resolver := hintResolverFunc(func(ctx context.Context, chain uint64, factory common.Address) (CreationHint, error) {
				calls++
				if _, ok := ctx.Deadline(); !ok || chain != 8453 || factory != hint.Factory {
					t.Fatal("unbounded or incorrect lookup")
				}
				switch mode {
				case "wrong factory":
					return CreationHint{Factory: common.HexToAddress("0x1"), TransactionHash: hint.TransactionHash}, nil
				case "missing":
					return CreationHint{}, nil
				case "error":
					return CreationHint{}, sentinel
				}
				return hint, nil
			})
			r, err := NewReaderWithCreationResolver(f, f, resolver, limits())
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Analyze(context.Background(), req)
			if calls != 1 || (result.Observation != nil) != (mode == "valid") || mode == "error" && !errors.Is(err, sentinel) {
				t.Fatalf("calls=%d result=%+v error=%v", calls, result, err)
			}
			if mode == "valid" {
				req.Evidence = result.Evidence
				result, err = r.Analyze(context.Background(), req)
				if err != nil || result.Observation == nil || calls != 1 {
					t.Fatal("saved creation candidate not reused", err)
				}
			}
		})
	}
}

// TestPermanentRatios separates temporary and permanent shares without rounding away unprotected dust.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentRatios(t *testing.T) {
	price := new(big.Int).Lsh(big.NewInt(1), 96)
	pos := func(n int64, kind string) Position {
		return Position{Lower: -60, Upper: 60, Liquidity: big.NewInt(n), Kind: kind, CanWeakenProtection: flag(false)}
	}
	locked := pos(3, "locked")
	until := time.Unix(1800000000, 0)
	locked.UnlockAt = &until
	open := pos(2, "withdrawable")
	open.CanWeakenProtection = nil
	v, reason, err := aggregate(price, []Position{pos(5, "permanent"), locked, open})
	if err != nil || reason != "" || *v.Token0.LockedPercentage != "30" || *v.Token0.PermanentlyProtectedPercentage != "50" || v.AllPositionsProtected || v.EarliestUnlockAt == nil || v.CanWeakenProtection == nil || *v.CanWeakenProtection {
		t.Fatalf("%+v %s %v", v, reason, err)
	}
	for _, weak := range []*bool{nil, flag(false), flag(true)} {
		p := pos(7, "permanent")
		p.CanWeakenProtection = weak
		v, _, err := aggregate(price, []Position{p, locked})
		if err != nil || !v.AllPositionsProtected || !reflect.DeepEqual(v.CanWeakenProtection, weak) {
			t.Fatalf("%+v %v", v, err)
		}
	}
	large := pos(1, "permanent")
	large.Liquidity = new(big.Int).Lsh(big.NewInt(1), 127)
	v, _, err = aggregate(price, []Position{large, pos(1, "withdrawable")})
	if err != nil || v.AllPositionsProtected || *v.Token0.PermanentlyProtectedPercentage != "99.999999999999999999" {
		t.Fatalf("%+v %v", v, err)
	}
	v, reason, err = aggregate(price, []Position{pos(1, "permanent"), pos(1, "")})
	if err != nil || v != nil || reason != "unsupported_custody" {
		t.Fatalf("%+v %s %v", v, reason, err)
	}
}
