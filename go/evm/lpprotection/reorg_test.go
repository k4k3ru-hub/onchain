package lpprotection

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

type historyForkRPC struct {
	*fakeRPC
	forked   bool
	event    types.Log
	switchAt uint64
	failAt   uint64
	failure  error
}

// HeaderByNumber switches canonical branches between checkpoint acquisitions.
//
// Version:
//   - 2026-09-23: Added.
func (f *historyForkRPC) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	if n.Uint64() == f.switchAt && !f.forked {
		f.forked = true
		f.approvalLogs = []types.Log{f.event}
	}
	if n.Uint64() == f.failAt && f.failure != nil {
		return nil, f.failure
	}
	h, err := f.fakeRPC.HeaderByNumber(ctx, n)
	if err != nil {
		return nil, err
	}
	if f.forked && n.Uint64() >= 30 {
		h = types.CopyHeader(h)
		h.Extra = []byte("new canonical branch")
	}
	return h, nil
}

// TestHistoryMidScanReorg prevents a new fork's checkpoint from endorsing an
// old fork's missing approvals, including when evaluation stops before its end.
//
// Version:
//   - 2026-09-23: Added.
func TestHistoryMidScanReorg(t *testing.T) {
	for _, failedTail := range []bool{false, true} {
		t.Run(map[bool]string{false: "final observation", true: "interrupted later range"}[failedTail], func(t *testing.T) {
			base, req, manager, owner := vaultFixture(t)
			req.Creation.BlockNumber = 2
			receipt := base.receipts[req.Creation.TxHash]
			receipt.BlockNumber = big.NewInt(2)
			for _, l := range receipt.Logs {
				l.BlockNumber = 2
			}
			operator := common.HexToAddress("0x700001")
			event := approvalEvent(manager, owner, operator, 1)
			event.BlockNumber = 35
			base.integer(manager, "isApprovedForAll(address,address)", big.NewInt(1), owner.Big(), operator.Big())
			f := &historyForkRPC{fakeRPC: base, event: event, switchAt: 79}
			lim := limits()
			lim.LogBlockRange = 40
			// Let the initial observation header succeed before injecting a later fault.
			if failedTail {
				f.failAt = 119
				f.failure = errors.New("later range unavailable")
				req.Principal.Snapshot.BlockNumber = 140
				base.header.Number = big.NewInt(140)
				base.hash = base.header.Hash()
				req.Principal.Snapshot.BlockHash = base.hash
			}
			reader, err := NewReader(f, lim)
			if err != nil {
				t.Fatal(err)
			}
			first, err := reader.Analyze(context.Background(), req)
			if err == nil || first.Observation != nil || first.Evidence == nil {
				t.Fatal("expected reorg rejection", err)
			}
			if len(first.Evidence.Histories()) != 0 || len(first.Evidence.Creations()) != 0 {
				t.Fatal("mixed-branch evidence survived")
			}
			f.failure = nil
			current, err := f.HeaderByNumber(context.Background(), base.header.Number)
			if err != nil {
				t.Fatal(err)
			}
			req.Principal.Snapshot.BlockHash = current.Hash()
			base.hash = current.Hash()
			encoded, err := json.Marshal(first.Evidence)
			if err != nil {
				t.Fatal(err)
			}
			req.Evidence, err = RestoreEvidence(encoded, lim.MaxResponseBytes)
			if err != nil {
				t.Fatal(err)
			}
			f.queries = nil
			second, err := reader.Analyze(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if second.Observation != nil || len(f.queries) == 0 || second.Positions[0].Reason != "operator_approval_active" {
				t.Fatalf("approval omitted after reorg: %+v queries=%d", second, len(f.queries))
			}
		})
	}
}

func reincludedReceiptFixture(t *testing.T) (*fakeRPC, Request, *types.Receipt) {
	t.Helper()
	f, req, _ := fixture(t, clliquidity.V3)
	first, err := run(t, f, req)
	if err != nil || first.Observation == nil {
		t.Fatal("baseline", err)
	}
	req.Evidence = first.Evidence
	encoded, err := json.Marshal(f.receipts[req.Creation.TxHash])
	if err != nil {
		t.Fatal(err)
	}
	var previous types.Receipt
	if err := json.Unmarshal(encoded, &previous); err != nil {
		t.Fatal(err)
	}
	f.header = types.CopyHeader(f.header)
	f.header.Extra = []byte("new canonical branch")
	f.hash = f.header.Hash()
	req.Principal.Snapshot.BlockHash = f.hash
	req.Creation.BlockNumber = 98
	req.Creation.BlockHash = common.HexToHash("0x9898")
	receipt := f.receipts[req.Creation.TxHash]
	receipt.BlockNumber = big.NewInt(98)
	receipt.BlockHash = req.Creation.BlockHash
	for _, l := range receipt.Logs {
		l.BlockNumber = 98
		l.BlockHash = req.Creation.BlockHash
	}
	return f, req, &previous
}

// TestReincludedReceipt reacquires a moved transaction once and persists the
// replacement without modifying request caches or requiring manual eviction.
//
// Version:
//   - 2026-09-23: Added.
func TestReincludedReceipt(t *testing.T) {
	for _, source := range []string{"evidence", "request"} {
		t.Run(source, func(t *testing.T) {
			f, req, previous := reincludedReceiptFixture(t)
			if source == "request" {
				req.Receipts = map[common.Hash]*types.Receipt{req.Creation.TxHash: previous}
			}
			result, err := run(t, f, req)
			if err != nil || result.Observation == nil || result.Metrics.AdditionalReceipts != 1 {
				t.Fatalf("recovery failed: %+v %v", result, err)
			}
			if previous.BlockNumber.Uint64() != 99 {
				t.Fatal("request receipt mutated")
			}
			req.Evidence = result.Evidence
			req.Receipts = nil
			reused, err := run(t, f, req)
			if err != nil || reused.Observation == nil || reused.Metrics.AdditionalReceipts != 0 {
				t.Fatalf("replacement not reused: %+v %v", reused, err)
			}
		})
	}
}

type headerFaultRPC struct {
	RPC
	number        uint64
	onCall, calls int
	failure       error
}

// HeaderByNumber injects a transient failure or reorg at one header read.
//
// Version:
//   - 2026-09-23: Added.
func (f *headerFaultRPC) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	h, err := f.RPC.HeaderByNumber(ctx, n)
	if err != nil || n.Uint64() != f.number {
		return h, err
	}
	f.calls++
	if f.calls != f.onCall {
		return h, nil
	}
	if f.failure != nil {
		return nil, f.failure
	}
	h = types.CopyHeader(h)
	h.Extra = []byte("reorg at verification")
	return h, nil
}

// TestHistoryPrefixFailure retains only the preceding verified range and retries
// the same failed range when a fresh prefix header cannot be acquired.
//
// Version:
//   - 2026-09-23: Added.
func TestHistoryPrefixFailure(t *testing.T) {
	base, req, _, _ := vaultFixture(t)
	failure := errors.New("prefix header unavailable")
	f := &headerFaultRPC{RPC: base, number: 39, onCall: 3, failure: failure}
	lim := limits()
	lim.LogBlockRange = 40
	r, err := NewReader(f, lim)
	if err != nil {
		t.Fatal(err)
	}
	first, err := r.Analyze(context.Background(), req)
	if !errors.Is(err, failure) || first.Observation != nil || first.Evidence == nil {
		t.Fatal("expected failed verification", err)
	}
	h := first.Evidence.Histories()
	if len(h) != 1 || h[0].Through == nil || h[0].Through.Number != 39 || h[0].Failure == nil || *h[0].Failure != (HistoryFailure{FromBlock: 40, ToBlock: 79, Attempts: 1, Reason: "acquisition_failed"}) {
		t.Fatalf("unverified history promoted: %+v", h)
	}
	req.Evidence = first.Evidence
	base.queries = nil
	second, err := r.Analyze(context.Background(), req)
	if err != nil || second.Observation == nil || len(base.queries) != 2 || base.queries[0].FromBlock.Uint64() != 40 || base.queries[0].ToBlock.Uint64() != 79 {
		t.Fatalf("failed range not resumed: %+v %v", second, err)
	}
}

// TestHistoryFinalReorg clears history even when the fork changes after its last
// checkpoint, before the final observation check.
//
// Version:
//   - 2026-09-23: Added.
func TestHistoryFinalReorg(t *testing.T) {
	base, req, _, _ := vaultFixture(t)
	f := &headerFaultRPC{RPC: base, number: 100, onCall: 4}
	r, err := NewReader(f, limits())
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.Analyze(context.Background(), req)
	if err == nil || result.Observation != nil || result.Evidence == nil || len(result.Evidence.Histories()) != 0 || len(result.Evidence.data.Acquired) != 0 {
		t.Fatalf("final reorg evidence survived: %+v %v", result, err)
	}
}

// TestHistoryBirthReorg verifies a nonzero creation anchor again before saving
// the first approval checkpoint.
//
// Version:
//   - 2026-09-23: Added.
func TestHistoryBirthReorg(t *testing.T) {
	base, req := creationSample(t)
	f := &headerFaultRPC{RPC: base, number: 51590853, onCall: 3}
	lim := limits()
	lim.LogBlockRange = 1000
	r, err := NewReaderWithCreationRPC(f, base, lim)
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.Analyze(context.Background(), req)
	if err == nil || result.Observation != nil || result.Evidence == nil || len(result.Evidence.Histories()) != 0 || len(result.Evidence.Creations()) != 0 || len(base.queries) != 1 {
		t.Fatalf("birth anchor reorg survived: %+v queries=%d %v", result, len(base.queries), err)
	}
}

type receiptFaultRPC struct {
	*fakeRPC
	failure error
}

// TransactionReceipt fails one acquisition without an internal retry.
//
// Version:
//   - 2026-09-23: Added.
func (f *receiptFaultRPC) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	r, err := f.fakeRPC.TransactionReceipt(ctx, hash)
	if f.failure != nil {
		return nil, f.failure
	}
	return r, err
}

// TestReincludedReceiptFailure evicts stale receipts even when replacement is
// unavailable and preserves the acquisition error and budgets.
//
// Version:
//   - 2026-09-23: Added.
func TestReincludedReceiptFailure(t *testing.T) {
	for _, mode := range []string{"rpc", "budget", "stale rpc response"} {
		t.Run(mode, func(t *testing.T) {
			base, req, previous := reincludedReceiptFixture(t)
			lim := limits()
			f := &receiptFaultRPC{fakeRPC: base}
			var expected error
			wantReceipts := 1
			current := base.receipts[req.Creation.TxHash]
			switch mode {
			case "rpc":
				expected = errors.New("receipt unavailable")
				f.failure = expected
			case "budget":
				expected = ErrBudget
				lim.MaxCalls = 1
				wantReceipts = 0
			case "stale rpc response":
				base.receipts[req.Creation.TxHash] = previous
			}
			r, err := NewReader(f, lim)
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Analyze(context.Background(), req)
			if err == nil || expected != nil && !errors.Is(err, expected) || result.Observation != nil || result.Metrics.AdditionalReceipts != wantReceipts || result.Metrics.Calls > lim.MaxCalls || result.Evidence == nil {
				t.Fatalf("unexpected replacement failure: %+v %v", result, err)
			}
			if _, exists := result.Evidence.data.Acquired["receipt/"+req.Creation.TxHash.Hex()]; exists {
				t.Fatal("stale receipt retained")
			}
			req.Evidence = result.Evidence
			base.receipts[req.Creation.TxHash] = current
			recovered, err := run(t, base, req)
			if err != nil || recovered.Observation == nil || recovered.Metrics.AdditionalReceipts != 1 {
				t.Fatalf("recovery after failure: %+v %v", recovered, err)
			}
		})
	}
}

// TestReceiptInvalidatesDependentProof removes a stale receipt's creation and
// history while retaining unrelated failure counts and leaving input untouched.
//
// Version:
//   - 2026-09-23: Added.
func TestReceiptInvalidatesDependentProof(t *testing.T) {
	f, req := creationSample(t)
	first, err := analyzeCreation(t, f, req)
	if err != nil || first.Observation == nil || len(first.Evidence.Creations()) != 1 {
		t.Fatal("baseline", err)
	}
	// A different custodian's valid genesis history must keep its retry ledger.
	unrelated := first.Evidence.Histories()[0]
	unrelated.Custodian = common.HexToAddress("0x900001")
	unrelated.StartBlock, unrelated.StartHash = 0, common.Hash{}
	unrelated.Through = &BlockReference{Number: 39, Hash: common.HexToHash("0x39")}
	unrelated.Operators = nil
	unrelated.Failure = &HistoryFailure{FromBlock: 40, ToBlock: 79, Attempts: 4, Reason: "acquisition_failed"}
	first.Evidence.data.Histories = append(first.Evidence.data.Histories, unrelated)
	req.Evidence = first.Evidence
	original := req.Creation
	req.Creation.BlockHash = common.HexToHash("0xabcdef")
	result, err := analyzeCreation(t, f, req)
	if err == nil || result.Observation != nil || result.Metrics.AdditionalReceipts != 1 || result.Evidence == nil || len(result.Evidence.Creations()) != 0 {
		t.Fatalf("dependent proof retained: %+v %v", result, err)
	}
	h := result.Evidence.Histories()
	if len(h) != 1 || h[0].Custodian != unrelated.Custodian || h[0].Failure == nil || *h[0].Failure != *unrelated.Failure {
		t.Fatalf("unrelated failure ledger changed: %+v", h)
	}
	if len(first.Evidence.Creations()) != 1 || len(first.Evidence.Histories()) != 2 {
		t.Fatal("input evidence mutated")
	}
	req.Creation = original
	req.Evidence = result.Evidence
	recovered, err := analyzeCreation(t, f, req)
	if err != nil || recovered.Observation == nil || len(recovered.Evidence.Creations()) != 1 {
		t.Fatalf("proof not rebuilt: %+v %v", recovered, err)
	}
}
