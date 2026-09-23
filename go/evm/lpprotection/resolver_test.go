package lpprotection

import (
	"context"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"testing"
)

type hintResolverFunc func(context.Context, uint64, common.Address) (CreationHint, error)

// ResolveCreationHint implements the injected LP analysis boundary.
//
// Version:
//   - 2026-09-23: Added.
func (f hintResolverFunc) ResolveCreationHint(c context.Context, n uint64, a common.Address) (CreationHint, error) {
	return f(c, n, a)
}

// TestCreationResolver verifies LP protection behavior and boundary conditions.
//
// Version:
//   - 2026-09-23: Added.
func TestCreationResolver(t *testing.T) {
	for _, mode := range []string{"valid", "supplied", "wrong_factory", "wrong_transaction", "failure", "missing"} {
		t.Run(mode, func(t *testing.T) {
			f, req := creationSample(t)
			hint := req.CreationHints[0]
			if mode != "supplied" {
				req.CreationHints = nil
			}
			calls := 0
			sentinel := errors.New("lookup unavailable")
			resolver := hintResolverFunc(func(ctx context.Context, n uint64, a common.Address) (CreationHint, error) {
				calls++
				if _, ok := ctx.Deadline(); !ok || n != 8453 || a != hint.Factory {
					t.Fatal("lookup identity/deadline")
				}
				h := hint
				switch mode {
				case "wrong_factory":
					h.Factory = common.HexToAddress("0x1")
				case "wrong_transaction":
					h.TransactionHash = common.HexToHash("0x1")
				case "failure":
					return h, sentinel
				case "missing":
					h.TransactionHash = common.Hash{}
				}
				return h, nil
			})
			lim := limits()
			lim.LogBlockRange = 1000
			r, err := NewReaderWithCreationResolver(f, f, resolver, lim)
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Analyze(context.Background(), req)
			if mode == "valid" || mode == "supplied" {
				if err != nil || result.Observation == nil || !result.Observation.AllPositionsProtected {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			} else if result.Observation != nil {
				t.Fatal("unverified candidate accepted")
			}
			if mode == "supplied" && calls != 0 || mode != "supplied" && calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if mode == "failure" && !errors.Is(err, sentinel) {
				t.Fatalf("lost cause: %v", err)
			}
		})
	}
}

// TestCreationReorgIsInspectable verifies LP protection behavior and boundary conditions.
//
// Version:
//   - 2026-09-23: Added.
func TestCreationReorgIsInspectable(t *testing.T) {
	f, req := creationSample(t)
	f.reorgNumber = req.Principal.Snapshot.BlockNumber
	result, err := analyzeCreation(t, f, req)
	if !errors.Is(err, ErrReorg) || result.Reason != "observation_reorg" || result.Observation != nil {
		t.Fatalf("lost reorg classification: %+v %v", result, err)
	}
}
