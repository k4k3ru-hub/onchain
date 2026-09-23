package lpprotection

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

// TestReceiptDiscovery reuses acquired evidence and stops history once all positions are explained.
//
// Version:
//   - 2026-09-23: Added.
func TestReceiptDiscovery(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "fetched creation receipt", true: "cached creation receipt"}[cached], func(t *testing.T) {
			f, req, _ := fixture(t, clliquidity.V3)
			f.historyErr = errors.New("history unavailable")
			if cached {
				req.Receipts = f.receipts
			}
			result, err := run(t, f, req)
			if err != nil || result.Observation == nil || result.Metrics.Methods["eth_getLogs"] != 0 || len(result.PositionIDs) != 1 {
				t.Fatalf("%+v %v", result, err)
			}
			if cached && result.Metrics.AdditionalReceipts != 0 {
				t.Fatal("cached receipt fetched again")
			}
		})
	}
	t.Run("other cached receipt", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		f.historyErr = errors.New("history unavailable")
		creation := f.receipts[req.Creation.TxHash]
		mint, inc := *creation.Logs[1], *creation.Logs[2]
		creation.Logs = creation.Logs[:1]
		hash := common.HexToHash("0x44")
		mint.TxHash = hash
		inc.TxHash = hash
		req.Receipts = map[common.Hash]*types.Receipt{hash: {Status: 1, BlockNumber: big.NewInt(99), BlockHash: mint.BlockHash, TxHash: hash, Logs: []*types.Log{&mint, &inc}}}
		result, err := run(t, f, req)
		if err != nil || result.Observation == nil || result.Metrics.Methods["eth_getLogs"] != 0 || result.Metrics.AdditionalReceipts != 1 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("history stops at complete coverage", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		creation := f.receipts[req.Creation.TxHash]
		full := *creation
		// Move the mint to another receipt so it must first be found in history.
		hash := common.HexToHash("0x44")
		mint, inc := *full.Logs[1], *full.Logs[2]
		mint.TxHash = hash
		inc.TxHash = hash
		creation.Logs = creation.Logs[:1]
		f.logs = []types.Log{mint}
		f.receipts[hash] = &types.Receipt{Status: 1, BlockNumber: big.NewInt(99), BlockHash: mint.BlockHash, TxHash: hash, Logs: []*types.Log{&mint, &inc}}
		lim := limits()
		lim.LogBlockRange = 1
		r, err := NewReader(f, lim)
		if err != nil {
			t.Fatal(err)
		}
		result, err := r.Analyze(context.Background(), req)
		if err != nil || result.Observation == nil || len(f.queries) != 1 || f.queries[0].ToBlock.Uint64() != 99 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("progress survives position read failure", func(t *testing.T) {
		f, req, manager := fixture(t, clliquidity.V3)
		f.put(manager, "positions(uint256)", []byte{0}, big.NewInt(1))
		result, err := run(t, f, req)
		if err == nil || result.Observation != nil || len(result.PositionIDs) != 1 || result.PositionIDs[0].Int64() != 1 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("cached removed mint", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		f.receipts[req.Creation.TxHash].Logs[1].Removed = true
		result, err := run(t, f, req)
		if err == nil || result.Observation != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
}
