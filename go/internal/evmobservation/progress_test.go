package evmobservation

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/k4k3ru-hub/onchain/go/evm"
)

type proofRPC struct {
	head    uint64
	logs    []types.Log
	err     error
	filter  func(context.Context)
	changed map[uint64]bool
	queries []ethereum.FilterQuery
}

func header(n uint64) evm.BlockHeader {
	return evm.BlockHeader{Number: n, Hash: common.BigToHash(new(big.Int).SetUint64(n))}
}

// LatestHeader returns a deterministic chain head.
//
// Version:
//   - 2026-10-01: Added.
func (r *proofRPC) LatestHeader(context.Context) (evm.BlockHeader, error) { return header(r.head), nil }

// HeaderByNumber returns the canonical hash or a controlled replacement.
//
// Version:
//   - 2026-10-01: Added.
func (r *proofRPC) HeaderByNumber(_ context.Context, n uint64) (evm.BlockHeader, error) {
	h := header(n)
	if r.changed[n] {
		h.Hash = common.HexToHash("ffff")
	}
	return h, nil
}

// FilterLogs returns controlled logs while recording the exact filter scope.
//
// Version:
//   - 2026-10-01: Added.
func (r *proofRPC) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	r.queries = append(r.queries, q)
	if r.filter != nil {
		r.filter(ctx)
	}
	return r.logs, r.err
}

// TestIdleProof verifies canonical idle ranges, event boundaries and reorg fencing.
//
// Version:
//   - 2026-10-01: Added.
func TestIdleProof(t *testing.T) {
	query := ethereum.FilterQuery{Addresses: []common.Address{common.HexToAddress("1"), common.HexToAddress("2")}, Topics: [][]common.Hash{nil, {common.HexToHash("ab")}}}
	base := Cursor{Block: 100, Hash: header(100).Hash}
	for _, name := range []string{"idle", "same_head", "same_block_end", "already_applied", "pending_same_block", "pending_new_block", "reorg_before", "reorg_during", "target_reorg", "rpc_failure", "removed", "range_limit", "regressed"} {
		t.Run(name, func(t *testing.T) {
			r := &proofRPC{head: 102, changed: map[uint64]bool{}}
			cursor := base
			wantAdvance, wantError := true, false
			switch name {
			case "same_head":
				r.head = 100
				wantAdvance = false
			case "same_block_end":
				r.head = 100
				cursor.HasLog = true
				cursor.Index = 1
			case "already_applied":
				cursor.HasLog = true
				cursor.Index = 1
				r.logs = []types.Log{{BlockNumber: 100, BlockHash: cursor.Hash, Index: 1}}
			case "pending_same_block":
				cursor.HasLog = true
				cursor.Index = 1
				r.logs = []types.Log{{BlockNumber: 100, BlockHash: cursor.Hash, Index: 2}}
				wantAdvance = false
			case "pending_new_block":
				r.logs = []types.Log{{BlockNumber: 101, BlockHash: header(101).Hash}}
				wantAdvance = false
			case "reorg_before":
				r.changed[100] = true
				wantAdvance, wantError = false, true
			case "reorg_during":
				r.filter = func(context.Context) { r.changed[100] = true }
				wantAdvance, wantError = false, true
			case "target_reorg":
				r.filter = func(context.Context) { r.changed[102] = true }
				wantAdvance, wantError = false, true
			case "rpc_failure":
				r.err = errors.New("offline")
				wantAdvance, wantError = false, true
			case "removed":
				r.logs = []types.Log{{BlockNumber: 101, BlockHash: header(101).Hash, Removed: true}}
				wantAdvance, wantError = false, true
			case "range_limit":
				r.head = 2000
				wantAdvance, wantError = false, true
			case "regressed":
				r.head = 99
				wantAdvance, wantError = false, true
			}
			var applied Journal
			if name == "already_applied" {
				applied.Record(r.logs[0])
			}
			next, advanced, err := Prove(t.Context(), r, query, cursor, applied.Snapshot())
			if advanced != wantAdvance || (err != nil) != wantError {
				t.Fatalf("advanced=%v err=%v", advanced, err)
			}
			if advanced && (next.Block != r.head || next.HasLog) {
				t.Fatal("incorrect confirmation cursor", next)
			}
			if r.err != nil && !errors.Is(err, r.err) {
				t.Fatal("error chain lost")
			}
			if len(r.queries) > 0 {
				q := r.queries[0]
				from := uint64(101)
				if cursor.HasLog {
					from = 100
				}
				if q.FromBlock.Uint64() != from || q.ToBlock.Uint64() != r.head || !reflect.DeepEqual(query.Addresses, q.Addresses) || !reflect.DeepEqual(query.Topics, q.Topics) {
					t.Fatal("scope changed", q)
				}
			}
		})
	}
}

// TestIdleRunnerFencesReplacement verifies cancellation, stale results and duplicate-head suppression.
//
// Version:
//   - 2026-10-01: Added.
func TestIdleRunnerFencesReplacement(t *testing.T) {
	for _, mode := range []string{"unchanged_head", "replaced", "disconnected", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			state := State{Cursor: Cursor{Block: 100, Hash: header(100).Hash}, Baseline: Cursor{Block: 100, Hash: header(100).Hash}, Epoch: 1, ReceivedAt: time.Unix(100, 0)}
			active := true
			applied := 0
			reads := 0
			r := &proofRPC{head: 102}
			r.filter = func(context.Context) {
				switch mode {
				case "replaced":
					state.Epoch++
					r.err = errors.New("old request failed")
				case "disconnected":
					active = false
				case "cancelled":
					cancel()
				}
			}
			err := Run(ctx, r, ethereum.FilterQuery{Addresses: []common.Address{common.HexToAddress("1")}}, time.Millisecond, func() (State, Evidence, bool) {
				reads++
				if reads > 8 {
					cancel()
				}
				return state, nil, active
			}, func(s State, at time.Time) bool {
				if s != state || !active || !at.After(s.ReceivedAt) {
					t.Fatal("invalid state applied")
				}
				applied++
				return true
			})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if mode == "unchanged_head" {
				want = 1
			}
			if applied != want {
				t.Fatalf("applied=%d want=%d", applied, want)
			}
		})
	}
}

// TestIdleProofRequiresEveryAppliedLog rejects gaps before the newest delivered log and orphaned inputs.
//
// Version:
//   - 2026-10-01: Added.
func TestIdleProofRequiresEveryAppliedLog(t *testing.T) {
	query := ethereum.FilterQuery{Addresses: []common.Address{common.HexToAddress("1")}}
	cursor := Cursor{Block: 100, Hash: header(100).Hash}
	first := types.Log{BlockNumber: 101, BlockHash: header(101).Hash, Index: 1}
	last := first
	last.Index = 2
	r := &proofRPC{head: 102, logs: []types.Log{first, last}}
	var journal Journal
	journal.Record(last)
	if _, advanced, err := Prove(t.Context(), r, query, cursor, journal.Snapshot()); err != nil || advanced {
		t.Fatal("missing earlier log confirmed", err)
	}
	journal.Record(first)
	if _, advanced, err := Prove(t.Context(), r, query, cursor, journal.Snapshot()); err != nil || !advanced {
		t.Fatal("complete evidence rejected", err)
	}
	r.logs = []types.Log{last}
	if _, advanced, err := Prove(t.Context(), r, query, cursor, journal.Snapshot()); err == nil || advanced {
		t.Fatal("orphaned applied log confirmed")
	}
	r.logs = []types.Log{first, last}
	r.logs[0].Data = []byte{99}
	if _, advanced, err := Prove(t.Context(), r, query, cursor, journal.Snapshot()); err != nil || advanced {
		t.Fatal("conflicting payload confirmed", err)
	}
	for i := uint(3); i < 4100; i++ {
		event := last
		event.Index = i
		journal.Record(event)
	}
	if len(journal.Snapshot()) != 4096 || journal.Snapshot().contains(first) {
		t.Fatal("journal unbounded or evicted log retained")
	}
}
