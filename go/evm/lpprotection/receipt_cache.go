package lpprotection

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func (e *evaluation) receiptCandidate(hash common.Hash) (*types.Receipt, bool) {
	if r, ok := e.receipts[hash]; ok {
		e.result.Metrics.CacheHits++
		return r, true
	}
	if r, ok := e.req.Receipts[hash]; ok {
		e.result.Metrics.CacheHits++
		return r, true
	}
	var r *types.Receipt
	cached := e.cachedEvidence("receipt/"+hash.Hex(), &r)
	return r, cached
}

func (e *evaluation) fetchReceipt(hash common.Hash) *types.Receipt {
	if e.err != nil {
		return nil
	}
	if e.receiptReads[hash] {
		e.err = fmt.Errorf("failed to acquire lp receipt: duplicate_acquisition=invalid")
		return nil
	}
	if e.result.Metrics.AdditionalReceipts >= e.r.limits.MaxReceipts {
		e.err = fmt.Errorf("failed to acquire lp receipt: %w", ErrBudget)
		return nil
	}
	if !e.attempt("eth_getTransactionReceipt") {
		return nil
	}
	if e.receiptReads == nil {
		e.receiptReads = make(map[common.Hash]bool)
	}
	e.receiptReads[hash] = true
	e.result.Metrics.AdditionalReceipts++
	r, err := e.r.rpc.TransactionReceipt(e.ctx, hash)
	if err != nil {
		e.err = fmt.Errorf("failed to acquire lp receipt: %w", err)
		return nil
	}
	if r != nil {
		for _, l := range r.Logs {
			if l != nil {
				e.response(160 + len(l.Topics)*32 + len(l.Data))
			}
		}
	}
	return r
}

func (e *evaluation) discardReceiptEvidence(hash common.Hash) {
	delete(e.receipts, hash)
	delete(e.evidence.data.Acquired, "receipt/"+hash.Hex())
	affected := make(map[common.Address]bool)
	creations := e.evidence.data.Creations[:0]
	for _, c := range e.evidence.data.Creations {
		if c.FactoryTransaction == hash || c.LockEvent.TxHash == hash {
			affected[c.Custodian] = true
			delete(e.starts, c.Custodian)
			delete(e.operatorChecks, c.Custodian)
			continue
		}
		creations = append(creations, c)
	}
	e.evidence.data.Creations = creations
	permanent := e.evidence.data.PermanentCustodies[:0]
	for _, c := range e.evidence.data.PermanentCustodies {
		if c.FactoryTransaction != hash {
			permanent = append(permanent, c)
		}
	}
	e.evidence.data.PermanentCustodies = permanent
	histories := e.evidence.data.Histories[:0]
	for _, h := range e.evidence.data.Histories {
		if !affected[h.Custodian] {
			histories = append(histories, h)
		}
	}
	e.evidence.data.Histories = histories
}
