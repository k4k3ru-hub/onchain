package lpprotection

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func (e *evaluation) block(number uint64, refresh bool) *types.Header {
	old := e.headers[number]
	if old != nil && !refresh {
		e.result.Metrics.CacheHits++
		return old
	}
	if !e.attempt("eth_getBlockByNumber") {
		return nil
	}
	h, err := e.r.rpc.HeaderByNumber(e.ctx, new(big.Int).SetUint64(number))
	if err != nil {
		e.err = fmt.Errorf("failed to read lp evidence block: %w", err)
		return nil
	}
	if h == nil || h.Number == nil || !h.Number.IsUint64() || h.Number.Uint64() != number {
		e.err = fmt.Errorf("failed to read lp evidence block: identity=invalid")
		return nil
	}
	e.response(600)
	if old != nil && old.Hash() != h.Hash() {
		e.invalidateEvidence()
		return nil
	}
	e.headers[number] = h
	return h
}

func (e *evaluation) canonical(ref BlockReference) bool {
	return e.checkCanonical(ref, false)
}

func (e *evaluation) canonicalFresh(ref BlockReference) bool {
	return e.checkCanonical(ref, true)
}

func (e *evaluation) checkCanonical(ref BlockReference, refresh bool) bool {
	h := e.block(ref.Number, refresh)
	if h == nil {
		return false
	}
	if h.Hash() != ref.Hash {
		e.invalidateEvidence()
		return false
	}
	return e.err == nil
}

func (e *evaluation) invalidateEvidence() {
	e.evidence.data.Creations = nil
	e.evidence.data.Histories = nil
	e.evidence.data.PermanentCustodies = nil
	// Contradictions retain their own canonical anchors. An unrelated reorg
	// must not silently clear a still-canonical authority conflict.
	clear(e.evidence.data.Acquired)
	e.err = fmt.Errorf("failed to verify lp evidence: %w", ErrReorg)
}

func (e *evaluation) historicalCode(address common.Address, hash common.Hash) []byte {
	key := "code/" + hash.Hex() + "/" + address.Hex()
	var code []byte
	if e.cachedEvidence(key, &code) {
		return code
	}
	if !e.attempt("eth_getCode") {
		return nil
	}
	var err error
	code, err = e.r.rpc.CodeAtHash(e.ctx, address, hash)
	if err != nil {
		e.err = fmt.Errorf("failed to read lp creation code: %w", err)
		return nil
	}
	e.response(len(code))
	e.cacheEvidence(key, code)
	return code
}

func (e *evaluation) historicalNonce(address common.Address, hash common.Hash) uint64 {
	key := "nonce/" + hash.Hex() + "/" + address.Hex()
	var nonce uint64
	if e.cachedEvidence(key, &nonce) {
		return nonce
	}
	if !e.attempt("eth_getTransactionCount") {
		return 0
	}
	var err error
	nonce, err = e.r.creationRPC.NonceAtHash(e.ctx, address, hash)
	if err != nil {
		e.err = fmt.Errorf("failed to read lp creation nonce: %w", err)
		return 0
	}
	e.response(8)
	e.cacheEvidence(key, nonce)
	return nonce
}

func (e *evaluation) creationTransaction(hash common.Hash) *types.Transaction {
	key := "transaction/" + hash.Hex()
	var tx *types.Transaction
	if !e.cachedEvidence(key, &tx) {
		if !e.attempt("eth_getTransactionByHash") {
			return nil
		}
		var pending bool
		var err error
		tx, pending, err = e.r.creationRPC.TransactionByHash(e.ctx, hash)
		if err != nil {
			e.err = fmt.Errorf("failed to read lp creation transaction: %w", err)
			return nil
		}
		if pending {
			e.err = fmt.Errorf("failed to read lp creation transaction: pending=invalid")
			return nil
		}
		if tx != nil {
			e.response(int(tx.Size()))
		}
		e.cacheEvidence(key, tx)
	}
	if tx == nil || tx.Hash() != hash || tx.ChainId().Cmp(new(big.Int).SetUint64(e.req.ChainID)) != 0 {
		e.err = fmt.Errorf("failed to verify lp creation transaction: identity=invalid")
		return nil
	}
	return tx
}

func (e *evaluation) creationReceipt(hash common.Hash) *types.Receipt {
	key := "receipt/" + hash.Hex()
	r, cached := e.receiptCandidate(hash)
	if !cached && e.err == nil {
		r = e.fetchReceipt(hash)
	}
	if e.err != nil {
		return nil
	}
	if r == nil || r.TxHash != hash || r.Status != types.ReceiptStatusSuccessful || r.BlockNumber == nil || !r.BlockNumber.IsUint64() || r.BlockNumber.Sign() <= 0 || r.BlockNumber.Uint64() > e.result.BlockNumber || r.BlockHash == (common.Hash{}) {
		e.err = fmt.Errorf("failed to verify lp creation receipt: identity=invalid")
		return nil
	}
	for _, l := range r.Logs {
		if l == nil || l.Removed || l.TxHash != hash || l.BlockHash != r.BlockHash || l.BlockNumber != r.BlockNumber.Uint64() {
			e.err = fmt.Errorf("failed to verify lp creation receipt: event=invalid")
			return nil
		}
	}
	e.cacheEvidence(key, r)
	e.receipts[hash] = r
	return r
}
