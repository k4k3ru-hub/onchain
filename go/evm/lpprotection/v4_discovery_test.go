package lpprotection

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func v4SeparateReceipt(f *fakeRPC, req Request) types.Log {
	creation := f.receipts[req.Creation.TxHash]
	modify := *creation.Logs[1]
	modify.TxHash = common.HexToHash("0x44")
	creation.Logs = creation.Logs[:1]
	f.logs = []types.Log{modify}
	f.receipts[modify.TxHash] = &types.Receipt{Status: 1, BlockNumber: new(big.Int).SetUint64(modify.BlockNumber), BlockHash: modify.BlockHash, TxHash: modify.TxHash, Logs: []*types.Log{&modify}}
	return modify
}

// TestV4Discovery prioritizes received evidence and scopes fallback history to the full Pool ID.
//
// Version:
//   - 2026-09-24: Added.
func TestV4Discovery(t *testing.T) {
	for _, source := range []string{"received log", "cached receipt", "history"} {
		t.Run(source, func(t *testing.T) {
			f, req, _ := v4Fixture(t)
			modify := v4SeparateReceipt(f, req)
			if source == "received log" {
				req.ModifyLiquidityLogs = []types.Log{modify}
			}
			if source == "cached receipt" {
				req.Receipts = map[common.Hash]*types.Receipt{modify.TxHash: f.receipts[modify.TxHash]}
			}
			lim := limits()
			lim.LogBlockRange = 1
			r, err := NewReader(f, lim)
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Analyze(context.Background(), req)
			if err != nil || result.Observation == nil || len(result.PositionIDs) != 1 {
				t.Fatalf("%+v %v", result, err)
			}
			if source == "history" {
				if len(f.queries) != 1 || f.queries[0].ToBlock.Uint64() != 99 {
					t.Fatal("history did not stop at complete coverage")
				}
			} else if len(f.queries) != 0 {
				t.Fatal("reusable evidence triggered history")
			}
			// A non-creation receipt must also survive persistence as a source
			// of ranges; PositionIDs alone cannot identify burned NFTs.
			req.Evidence = result.Evidence
			req.Receipts, req.ModifyLiquidityLogs = nil, nil
			f.receipts, f.logs, f.queries = nil, nil, nil
			result, err = r.Analyze(context.Background(), req)
			if err != nil || result.Observation == nil || result.Metrics.AdditionalReceipts != 0 || len(f.queries) != 0 {
				t.Fatalf("restored=%+v %v", result, err)
			}
		})
	}
	t.Run("other pool in same receipt", func(t *testing.T) {
		f, req, _ := v4Fixture(t)
		r := f.receipts[req.Creation.TxHash]
		other := *r.Logs[1]
		other.Topics = append([]common.Hash(nil), other.Topics...)
		other.Topics[1][0] ^= 1 // Same final 20 bytes, a different V4 pool.
		other.Index++
		r.Logs = append(r.Logs, &other)
		result, err := run(t, f, req)
		if err != nil || result.Observation == nil || len(result.PositionIDs) != 1 {
			t.Fatalf("%+v %v", result, err)
		}
		req.ModifyLiquidityLogs = []types.Log{other}
		result, err = run(t, f, req)
		if err == nil || result.Observation != nil {
			t.Fatal("explicit other-pool input accepted")
		}
	})
	t.Run("malformed removed event", func(t *testing.T) {
		f, req, _ := v4Fixture(t)
		f.receipts[req.Creation.TxHash].Logs[1].Removed = true
		result, err := run(t, f, req)
		if err == nil || result.Observation != nil {
			t.Fatal("removed event accepted")
		}
	})
}

// TestV4Burn requires a known canonical range and explicit core zero to omit an NFT.
//
// Version:
//   - 2026-09-24: Added.
func TestV4Burn(t *testing.T) {
	t.Run("burn with core zero", func(t *testing.T) {
		f, req, d := v4Fixture(t)
		req.PositionIDs = []*big.Int{big.NewInt(1), big.NewInt(2)}
		v4PutPosition(t, f, req, d, 1, -60, 60, new(big.Int))
		v4PutPosition(t, f, req, d, 2, -60, 60, big.NewInt(1000000))
		// Calling the cleared position getter would return invalid data here.
		// Only the independently read core zero allows skipping it.
		f.put(d.PositionManager, "getPoolAndPositionInfo(uint256)", nil, big.NewInt(1))
		result, err := run(t, f, req)
		if err != nil || result.Observation == nil || len(result.Positions) != 1 || result.Positions[0].ID.Int64() != 2 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("missing range is unresolved", func(t *testing.T) {
		f, req, d := v4Fixture(t)
		req.PositionIDs = []*big.Int{big.NewInt(2)}
		f.put(d.PositionManager, "getPoolAndPositionInfo(uint256)", make([]byte, 192), big.NewInt(2))
		result, err := run(t, f, req)
		if err == nil || result.Observation != nil {
			t.Fatal("missing range classified as burned")
		}
	})
	t.Run("getter revert is not zero", func(t *testing.T) {
		f, req, _ := v4Fixture(t)
		sentinel := errors.New("position getter reverted")
		r, err := NewReader(&v4WithdrawalRPC{fakeRPC: f, getterErr: sentinel}, limits())
		if err != nil {
			t.Fatal(err)
		}
		result, err := r.Analyze(context.Background(), req)
		if !errors.Is(err, sentinel) || result.Observation != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

// TestV4Budgets keeps discovery and withdrawal within the existing acquisition limits.
//
// Version:
//   - 2026-09-24: Added.
func TestV4Budgets(t *testing.T) {
	for _, mode := range []string{"calls", "receipts", "response bytes", "combined input logs", "positions"} {
		t.Run(mode, func(t *testing.T) {
			f, req, _ := v4Fixture(t)
			lim := limits()
			switch mode {
			case "calls":
				lim.MaxCalls = 3
			case "receipts":
				lim.MaxReceipts = 1
				v4SeparateReceipt(f, req)
			case "response bytes":
				lim.MaxResponseBytes = 1000
			case "combined input logs":
				lim.MaxLogs = 1
				req.MintLogs = []types.Log{{}}
				req.ModifyLiquidityLogs = []types.Log{{}}
			case "positions":
				lim.MaxPositions = 1
				req.PositionIDs = []*big.Int{big.NewInt(1), big.NewInt(2)}
			}
			r, err := NewReader(f, lim)
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Analyze(context.Background(), req)
			if !errors.Is(err, ErrBudget) || result.Observation != nil || result.Reason != "acquisition_budget_exceeded" || result.Metrics.Calls > lim.MaxCalls || result.Metrics.AdditionalReceipts > lim.MaxReceipts {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}
