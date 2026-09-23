package lpprotection

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

type permanentAnchorRPC struct {
	*creationFake
	target  uint64
	reads   int
	failure error
}

// HeaderByNumber changes a historical anchor only at the final fresh check.
//
// Version:
//   - 2026-09-24: Added.
func (f *permanentAnchorRPC) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	h, err := f.creationFake.HeaderByNumber(ctx, n)
	if n.Uint64() == f.target {
		f.reads++
		if f.reads == 2 {
			if f.failure != nil {
				return nil, f.failure
			}
			h = types.CopyHeader(h)
			h.Extra = []byte("reorg during evaluation")
		}
	}
	return h, err
}

// TestPermanentFinalAnchors requires fresh birth/pool block checks before publishing protection.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentFinalAnchors(t *testing.T) {
	for _, anchor := range []string{"birth", "pool"} {
		for _, rpcFailure := range []bool{false, true} {
			f, req := permanentSample(t)
			target := uint64(50940130)
			if anchor == "pool" {
				target = req.Creation.BlockNumber
			}
			wrapped := &permanentAnchorRPC{creationFake: f, target: target}
			sentinel := errors.New("anchor unavailable")
			if rpcFailure {
				wrapped.failure = sentinel
			}
			r, err := NewReaderWithCreationRPC(wrapped, wrapped, limits())
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Analyze(context.Background(), req)
			if wrapped.reads != 2 || result.Observation != nil || err == nil {
				t.Fatalf("target=%d result=%+v error=%v", target, result, err)
			}
			if rpcFailure {
				if !errors.Is(err, sentinel) {
					t.Fatal("lost RPC cause", err)
				}
			} else if !errors.Is(err, ErrReorg) || len(result.Evidence.PermanentCustodies()) != 0 {
				t.Fatalf("stale proof retained: %+v %v", result, err)
			}
		}
	}
}

// TestPermanentAcquisitionLimits stops before any configured call or receipt budget is exceeded.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentAcquisitionLimits(t *testing.T) {
	for _, mode := range []string{"calls", "receipts", "bytes", "cancellation"} {
		t.Run(mode, func(t *testing.T) {
			f, req := permanentSample(t)
			lim := limits()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "calls":
				lim.MaxCalls = 23
			case "receipts":
				lim.MaxReceipts = 1
			case "bytes":
				lim.MaxResponseBytes = 70000
			case "cancellation":
				cancel()
			}
			r, err := NewReaderWithCreationRPC(f, f, lim)
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Analyze(ctx, req)
			want := ErrBudget
			if mode == "cancellation" {
				want = context.Canceled
			}
			if !errors.Is(err, want) || result.Observation != nil || result.Metrics.Calls > lim.MaxCalls || result.Metrics.AdditionalReceipts > lim.MaxReceipts {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}
