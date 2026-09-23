package lpprotection

import (
	"bytes"
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type evaluation struct {
	r                                *Reader
	ctx                              context.Context
	req                              Request
	result                           *Result
	err                              error
	manager, factory, token0, token1 common.Address
	third                            *big.Int
	calls                            map[string][]byte
	codes                            map[common.Address][]byte
	receipts                         map[common.Hash]*types.Receipt
	receiptReads                     map[common.Hash]bool
	operatorChecks                   map[common.Address]string
	logCount                         int
	evidence                         *Evidence
	headers                          map[uint64]*types.Header
	starts                           map[common.Address]BlockReference
	v4StateView                      common.Address
	v4Ranges                         map[string][2]int32
	permanentAnchors                 map[uint64]common.Hash
}

func (e *evaluation) logs(q ethereum.FilterQuery) []types.Log {
	if !e.attempt("eth_getLogs") {
		return nil
	}
	logs, err := e.r.rpc.FilterLogs(e.ctx, q)
	if err != nil {
		e.err = fmt.Errorf("failed to read lp events: %w", err)
		return nil
	}
	if len(logs) > e.r.limits.MaxLogs-e.logCount {
		e.err = fmt.Errorf("failed to read lp events: %w: logs=too_long", ErrBudget)
		return nil
	}
	e.logCount += len(logs)
	for _, l := range logs {
		e.response(160 + len(l.Topics)*32 + len(l.Data))
		if l.Removed || l.BlockHash == (common.Hash{}) || l.TxHash == (common.Hash{}) || new(big.Int).SetUint64(l.BlockNumber).Cmp(q.FromBlock) < 0 || new(big.Int).SetUint64(l.BlockNumber).Cmp(q.ToBlock) > 0 {
			e.err = fmt.Errorf("failed to read lp events: identity=invalid")
			return nil
		}
	}
	return logs
}

func (e *evaluation) attempt(method string) bool {
	if e.err != nil {
		return false
	}
	if err := e.ctx.Err(); err != nil {
		e.err = fmt.Errorf("failed to acquire lp evidence: %w", err)
		return false
	}
	if e.result.Metrics.Calls >= e.r.limits.MaxCalls {
		e.err = fmt.Errorf("failed to acquire lp evidence: %w", ErrBudget)
		return false
	}
	e.result.Metrics.Calls++
	e.result.Metrics.Methods[method]++
	return true
}

func (e *evaluation) response(size int) {
	e.result.Metrics.ResponseBytes += size
	if e.result.Metrics.ResponseBytes > e.r.limits.MaxResponseBytes && e.err == nil {
		e.err = fmt.Errorf("failed to acquire lp evidence: %w: response_bytes=out_of_range", ErrBudget)
	}
}

func query(signature string, args ...*big.Int) []byte {
	b := append([]byte{}, crypto.Keccak256([]byte(signature))[:4]...)
	for _, arg := range args {
		v := new(big.Int).Set(arg)
		if v.Sign() < 0 {
			v.Add(v, new(big.Int).Lsh(big.NewInt(1), 256))
		}
		b = append(b, v.FillBytes(make([]byte, 32))...)
	}
	return b
}

func (e *evaluation) call(from, to common.Address, data []byte) []byte {
	key := string(from[:]) + string(to[:]) + string(data)
	if b, ok := e.calls[key]; ok {
		e.result.Metrics.CacheHits++
		return b
	}
	if !e.attempt("eth_call") {
		return nil
	}
	b, err := e.r.rpc.CallContractAtHash(e.ctx, ethereum.CallMsg{From: from, To: &to, Data: data}, e.result.BlockHash)
	if err != nil {
		e.err = fmt.Errorf("failed to read lp contract: %w: address=%q", err, to.Hex())
		return nil
	}
	e.response(len(b))
	e.calls[key] = b
	return b
}

func (e *evaluation) words(to common.Address, signature string, count int, args ...*big.Int) []*big.Int {
	b := e.call(common.Address{}, to, query(signature, args...))
	values := make([]*big.Int, count)
	for i := range values {
		values[i] = new(big.Int)
	}
	if e.err != nil {
		return values
	}
	if len(b) != 32*count {
		e.err = fmt.Errorf("failed to decode lp contract: response=invalid address=%q", to.Hex())
		return values
	}
	for i := range values {
		values[i].SetBytes(b[32*i : 32*(i+1)])
	}
	return values
}

func (e *evaluation) word(to common.Address, signature string, args ...*big.Int) *big.Int {
	return e.words(to, signature, 1, args...)[0]
}

func (e *evaluation) address(to common.Address, signature string, args ...*big.Int) common.Address {
	v := e.word(to, signature, args...)
	if v.BitLen() > 160 && e.err == nil {
		e.err = fmt.Errorf("failed to decode lp address: value=invalid")
	}
	return common.BigToAddress(v)
}

func (e *evaluation) code(address common.Address) []byte {
	if code, ok := e.codes[address]; ok {
		e.result.Metrics.CacheHits++
		return code
	}
	if !e.attempt("eth_getCode") {
		return nil
	}
	code, err := e.r.rpc.CodeAtHash(e.ctx, address, e.result.BlockHash)
	if err != nil {
		e.err = fmt.Errorf("failed to read lp custody code: %w: address=%q", err, address.Hex())
		return nil
	}
	e.response(len(code))
	e.codes[address] = code
	return code
}

func (e *evaluation) receipt(log types.Log) *types.Receipt {
	r, cached := e.receiptCandidate(log.TxHash)
	if e.err != nil {
		return nil
	}
	if cached && r != nil && r.TxHash == log.TxHash && r.Status == types.ReceiptStatusSuccessful && r.BlockNumber != nil && r.BlockNumber.IsUint64() && r.BlockHash != (common.Hash{}) && (r.BlockHash != log.BlockHash || r.BlockNumber.Uint64() != log.BlockNumber) {
		// A transaction may be re-included at a different block. Discard its
		// dependent proof before acquisition, including when the budget or RPC
		// prevents fetching a replacement in this evaluation.
		e.discardReceiptEvidence(log.TxHash)
		r = e.fetchReceipt(log.TxHash)
	} else if !cached {
		r = e.fetchReceipt(log.TxHash)
	}
	if e.err != nil {
		return nil
	}
	if r == nil || r.Status != types.ReceiptStatusSuccessful || r.TxHash != log.TxHash || r.BlockHash != log.BlockHash || r.BlockNumber == nil || !r.BlockNumber.IsUint64() || r.BlockNumber.Uint64() != log.BlockNumber {
		e.err = fmt.Errorf("failed to validate lp receipt: identity=invalid")
		return nil
	}
	for _, l := range r.Logs {
		if l != nil && !l.Removed && l.Address == log.Address && l.Index == log.Index && l.BlockHash == log.BlockHash && l.TxHash == log.TxHash && l.BlockNumber == log.BlockNumber && equalTopics(l.Topics, log.Topics) && bytes.Equal(l.Data, log.Data) {
			e.cacheEvidence("receipt/"+log.TxHash.Hex(), r)
			e.receipts[log.TxHash] = r
			return r
		}
	}
	e.err = fmt.Errorf("failed to validate lp receipt: event=empty")
	return nil
}

func equalTopics(a, b []common.Hash) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (e *evaluation) header() {
	if !e.attempt("eth_getBlockByNumber") {
		return
	}
	h, err := e.r.rpc.HeaderByNumber(e.ctx, new(big.Int).SetUint64(e.result.BlockNumber))
	if err != nil {
		e.err = fmt.Errorf("failed to verify lp observation: %w", err)
		return
	}
	if h == nil || h.Number == nil || !h.Number.IsUint64() || h.Number.Uint64() != e.result.BlockNumber {
		e.err = fmt.Errorf("failed to verify lp observation: block=invalid")
		return
	}
	if h.Hash() != e.result.BlockHash {
		e.invalidateEvidence()
		return
	}
	if h.Time != uint64(e.result.BlockTime.Unix()) {
		e.err = fmt.Errorf("failed to verify lp observation: block=invalid")
	}
}

func signed(v *big.Int) *big.Int {
	n := new(big.Int).Set(v)
	if n.Bit(255) != 0 {
		n.Sub(n, new(big.Int).Lsh(big.NewInt(1), 256))
	}
	return n
}

func ordinary(address common.Address, code []byte) bool {
	return len(code) == 0 && address.Big().Cmp(big.NewInt(65535)) > 0 && address != common.HexToAddress("0x000000000000000000000000000000000000dead")
}

func flag(v bool) *bool { return &v }
