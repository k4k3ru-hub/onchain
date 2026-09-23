package lpprotection

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// TestHistoryProgress preserves gaps/retry counts across persistence and resumes
// only from the failed range. Four failed attempts stop further history calls.
//
// Version:
//   - 2026-09-23: Added.
func TestHistoryProgress(t *testing.T) {
	f, req := creationSample(t)
	f.failFrom = 51592853
	for attempt := 1; attempt <= 4; attempt++ {
		f.queries = nil
		result, err := analyzeCreation(t, f, req)
		if err == nil || result.Observation != nil || result.Evidence == nil {
			t.Fatalf("failure became a verdict: %+v %v", result, err)
		}
		h := result.Evidence.Histories()[0]
		if h.Through == nil || h.Through.Number != 51592852 || h.Failure == nil || h.Failure.Attempts != uint8(attempt) || h.Failure.FromBlock != 51592853 || h.Failure.ToBlock != 51593852 {
			t.Fatalf("wrong progress: %+v", h)
		}
		if attempt > 1 && (len(f.queries) != 1 || f.queries[0].FromBlock.Uint64() != f.failFrom) {
			t.Fatal("refetched completed range or skipped gap")
		}
		b, encodeErr := json.Marshal(result.Evidence)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		req.Evidence, encodeErr = RestoreEvidence(b, limits().MaxResponseBytes)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
	}
	f.queries = nil
	f.failFrom = 0
	result, err := analyzeCreation(t, f, req)
	if !errors.Is(err, ErrHistoryAbandoned) || result.Observation != nil || len(f.queries) != 0 {
		t.Fatalf("abandoned gap was reset: %+v %v", result, err)
	}
}

// TestHistoryRecovery keeps partial progress and finishes an interrupted scan.
//
// Version:
//   - 2026-09-23: Added.
func TestHistoryRecovery(t *testing.T) {
	f, req := creationSample(t)
	f.failFrom = 51592853
	first, err := analyzeCreation(t, f, req)
	if err == nil {
		t.Fatal("expected gap")
	}
	req.Evidence = first.Evidence
	f.failFrom = 0
	f.queries = nil
	result, err := analyzeCreation(t, f, req)
	if err != nil || result.Observation == nil || len(f.queries) != 5 || f.queries[0].FromBlock.Uint64() != 51592853 || result.Evidence.Histories()[0].Failure != nil {
		t.Fatalf("resume failed: %+v %v", result, err)
	}
	// Analyze must not mutate the previous result while finishing its successor.
	if first.Evidence.Histories()[0].Failure == nil {
		t.Fatal("input evidence mutated")
	}
}

// TestHistoryCurrentState includes birth-block approvals and rereads their
// current value after cache restore, even when there are no additional logs.
//
// Version:
//   - 2026-09-23: Added.
func TestHistoryCurrentState(t *testing.T) {
	f, req := creationSample(t)
	manager := common.HexToAddress("0xe1f8cd9ac4e4a65f54f38a5cdafca44f6dd68b53")
	owner := common.HexToAddress("0xdbf7a3d301e39871e6644a17fb1af1a3889078af")
	event := approvalEvent(manager, owner, common.HexToAddress("0x700001"), 0)
	event.BlockNumber = req.Creation.BlockNumber
	event.BlockHash = req.Creation.BlockHash
	f.approvals = []types.Log{event}
	f.currentApproval = 1
	result, err := analyzeCreation(t, f, req)
	if err != nil || result.Observation != nil || result.Positions[0].Reason != "operator_approval_active" || result.Positions[0].CanWeakenProtection != nil {
		t.Fatalf("ignored birth operator: %+v %v", result, err)
	}
	req.Evidence = result.Evidence
	f.currentApproval = 0
	f.queries = nil
	result, err = analyzeCreation(t, f, req)
	if err != nil || result.Observation == nil || len(f.queries) != 0 || len(result.Evidence.Histories()[0].Operators) != 1 {
		t.Fatalf("current state not checked: %+v %v", result, err)
	}
}

// TestHistoryInvalidation rejects stale proofs after canonicality or model changes.
//
// Version:
//   - 2026-09-23: Added.
func TestHistoryInvalidation(t *testing.T) {
	for _, number := range []uint64{44515546, 51590853, 51592852} {
		f, req := creationSample(t)
		f.failFrom = 51592853
		first, _ := analyzeCreation(t, f, req)
		req.Evidence = first.Evidence
		f.failFrom = 0
		f.reorgNumber = number
		f.queries = nil
		result, err := analyzeCreation(t, f, req)
		if err == nil || result.Observation != nil || len(result.Evidence.Histories()) != 0 || len(result.Evidence.Creations()) != 0 {
			t.Fatalf("stale reorg evidence: number=%d %+v %v", number, result, err)
		}
	}
	f, req := creationSample(t)
	result, err := analyzeCreation(t, f, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(result.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{[]byte(strings.Replace(string(b), ModelVersion, "lp-protection-20260923-v2", 1)), []byte(`{"complete":true}`), append(append([]byte{}, b...), []byte(` {}`)...)} {
		if _, err := RestoreEvidence(bad, len(b)*2); err == nil {
			t.Fatal("accepted invalid persisted evidence")
		}
	}
	if _, err := RestoreEvidence(b, len(b)-1); !errors.Is(err, ErrBudget) {
		t.Fatal("missing restore size bound", err)
	}
}

// TestCreationConstructor excludes fake init code that would return an otherwise
// accepted runtime, and proves the rule is not keyed to the sampled address.
//
// Version:
//   - 2026-09-23: Added.
func TestCreationConstructor(t *testing.T) {
	f, req := creationSample(t)
	original := f.data.Transactions[req.CreationHints[0].TransactionHash]
	fields, ok := match(common.FromHex(f.data.Codes[strings.ToLower(req.CreationHints[0].Factory.Hex()+req.Principal.Snapshot.BlockHash.Hex())]), "clFactory")
	if !ok {
		t.Fatal("fixture runtime")
	}
	// This key belongs only to an offline synthetic transaction. Never transmitted.
	key, err := crypto.HexToECDSA(strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	origin := crypto.PubkeyToAddress(key.PublicKey)
	data := append([]byte(nil), original.Data()...)
	copy(data[4:24], origin[:])
	salt := crypto.Keccak256Hash(common.LeftPadBytes(origin[:], 32), data[4:36])
	proxy := crypto.CreateAddress2(*original.To(), salt, crypto.Keccak256(common.FromHex("0x67363d3d37363d34f03d5260086018f3")))
	factory := crypto.CreateAddress(proxy, 1)
	sign := func(d []byte) *types.Transaction {
		tx, err := types.SignTx(types.NewTx(&types.LegacyTx{To: original.To(), Data: d, Gas: 16000000}), types.LatestSignerForChainID(big.NewInt(8453)), key)
		if err != nil {
			t.Fatal(err)
		}
		return tx
	}
	if _, _, _, ok := factoryDeployment(sign(data), factory, fields); !ok {
		t.Fatal("rule incorrectly address-specific")
	}
	for _, offset := range []int{100, 100 + 12987, 100 + 12988 + 64, 36, 24} {
		changed := append([]byte(nil), data...)
		changed[offset] ^= 1
		if _, _, _, ok := factoryDeployment(sign(changed), factory, fields); ok {
			t.Fatalf("accepted unsupported constructor/argument at %d", offset)
		}
	}
}

// TestCreationComposition keeps the old constructor and binds optional services
// explicitly without performing acquisition during construction.
//
// Version:
//   - 2026-09-23: Added.
func TestCreationComposition(t *testing.T) {
	f, req := creationSample(t)
	r, err := NewReaderWithCreationRPC(f, f, limits())
	if err != nil || r.creationRPC != f || r.rpc != f || f.attempts != 0 {
		t.Fatal("composition failed", err)
	}
	if _, err := NewReaderWithCreationRPC(f, nil, limits()); err == nil {
		t.Fatal("nil creation rpc")
	}
	old, err := NewReader(f, limits())
	if err != nil {
		t.Fatal(err)
	}
	result, err := old.Analyze(context.Background(), req)
	if !errors.Is(err, ErrBudget) || result.Observation != nil || len(f.queries) != 0 {
		t.Fatal("legacy constructor silently enabled birth hints", err)
	}
	lim := limits()
	lim.MaxCalls = 40
	bounded, err := NewReaderWithCreationRPC(f, f, lim)
	if err != nil {
		t.Fatal(err)
	}
	result, err = bounded.Analyze(context.Background(), req)
	if !errors.Is(err, ErrBudget) || result.Observation != nil || result.Metrics.Calls > 40 || result.Evidence == nil {
		t.Fatal("acquisition budget bypassed", err)
	}
}

func advanceCreationSample(f *creationFake, req *Request, number uint64) {
	old := req.Principal.Snapshot.BlockHash
	h := types.CopyHeader(f.data.Headers[req.Principal.Snapshot.BlockNumber])
	h.Number = new(big.Int).SetUint64(number)
	h.Time += 2 * (number - req.Principal.Snapshot.BlockNumber)
	f.data.Headers[number] = h
	for key, value := range f.data.Codes {
		if strings.HasSuffix(key, old.Hex()) {
			f.data.Codes[strings.TrimSuffix(key, old.Hex())+h.Hash().Hex()] = value
		}
	}
	for key, value := range f.data.Calls {
		if strings.HasSuffix(key, old.Hex()) {
			f.data.Calls[strings.TrimSuffix(key, old.Hex())+h.Hash().Hex()] = value
		}
	}
	req.Principal.Snapshot.BlockNumber = number
	req.Principal.Snapshot.BlockHash = h.Hash()
	req.Principal.Snapshot.BlockTime = time.Unix(int64(h.Time), 0).UTC()
}

// TestHistoryNewObservation fetches only the new interval and preserves failed
// interval identity when the observation moves forward.
//
// Version:
//   - 2026-09-23: Added.
func TestHistoryNewObservation(t *testing.T) {
	for _, gap := range []bool{false, true} {
		f, req := creationSample(t)
		if gap {
			f.failFrom = 51592853
		}
		first, err := analyzeCreation(t, f, req)
		if (err != nil) != gap {
			t.Fatal("unexpected first evaluation", err)
		}
		req.Evidence = first.Evidence
		advanceCreationSample(f, &req, 51596910)
		f.queries = nil
		result, err := analyzeCreation(t, f, req)
		if gap {
			if err == nil || result.Observation != nil || len(f.queries) != 1 || f.queries[0].FromBlock.Uint64() != 51592853 || result.Evidence.Histories()[0].Failure.Attempts != 2 {
				t.Fatal("new observation reset or skipped the gap", err)
			}
		} else if err != nil || result.Observation == nil || len(f.queries) != 1 || f.queries[0].FromBlock.Uint64() != 51596897 || f.queries[0].ToBlock.Uint64() != 51596910 {
			t.Fatal("not a contiguous delta", err)
		}
	}
}
