package lpprotection

import (
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var ErrHistoryAbandoned = errors.New("lp operator history retry limit reached")

// The canonical managers emit every ApprovalForAll update. Only a verified
// first incarnation permits a nonzero start. A checkpoint represents contiguous
// successful queries, including the creation block, never received live events.
func (e *evaluation) operatorsClear(p *Position) bool {
	p.Reason = "operator_history_unresolved"
	if e.err != nil {
		return false
	}
	if reason, ok := e.operatorChecks[p.Owner]; ok {
		e.result.Metrics.CacheHits++
		p.Reason = reason
		return reason == ""
	}
	start := e.starts[p.Owner]
	managerHash := crypto.Keccak256Hash(e.code(e.manager))
	if e.err != nil {
		return false
	}
	h := OperatorHistory{Manager: e.manager, Custodian: p.Owner, ManagerCodeHash: managerHash, CustodianCodeHash: p.CodeHash, StartBlock: start.Number, StartHash: start.Hash}
	index := -1
	for i, previous := range e.evidence.data.Histories {
		if previous.Manager == e.manager && previous.Custodian == p.Owner {
			index = i
			if previous.ManagerCodeHash == managerHash && previous.CustodianCodeHash == p.CodeHash && previous.StartBlock == start.Number && previous.StartHash == start.Hash && (previous.Through == nil || previous.Through.Number <= e.result.BlockNumber) {
				h = previous
			}
			break
		}
	}
	if h.Through != nil && !e.canonical(*h.Through) {
		return false
	}
	if index < 0 {
		if len(e.evidence.data.Histories) >= e.r.limits.MaxPositions {
			e.err = fmt.Errorf("failed to retain lp operators: %w: histories=too_long", ErrBudget)
			return false
		}
		index = len(e.evidence.data.Histories)
		e.evidence.data.Histories = append(e.evidence.data.Histories, h)
	}
	save := func() {
		if index < len(e.evidence.data.Histories) {
			e.evidence.data.Histories[index] = h
		}
	}
	save()
	from := h.StartBlock
	done := false
	if h.Through != nil {
		done = h.Through.Number == e.result.BlockNumber
		from = h.Through.Number + 1
	}
	if h.Failure != nil && h.Failure.Attempts >= 4 {
		e.err = fmt.Errorf("failed to verify lp operators: %w: from_block=%d to_block=%d", ErrHistoryAbandoned, h.Failure.FromBlock, h.Failure.ToBlock)
		return false
	}
	// Each chunk needs logs, two tail headers and a fresh prefix/birth anchor.
	// Only the first genesis chunk has no prefix anchor to recheck. Reserve
	// current operator reads and the final observation header as well.
	remaining := e.r.limits.MaxCalls - e.result.Metrics.Calls - len(h.Operators) - 1
	if h.Through == nil && h.StartBlock == 0 {
		remaining++
	}
	if !done && (from > e.result.BlockNumber || remaining < 4 || (e.result.BlockNumber-from)/e.r.limits.LogBlockRange >= uint64(remaining/4)) {
		e.err = fmt.Errorf("failed to verify lp operators: %w: history_range=too_long", ErrBudget)
		return false
	}
	event := crypto.Keccak256Hash([]byte("ApprovalForAll(address,address,bool)"))
	ownerTopic := common.BytesToHash(p.Owner[:])
	for !done {
		to := e.result.BlockNumber
		if to-from >= e.r.limits.LogBlockRange {
			to = from + e.r.limits.LogBlockRange - 1
		}
		attempts := uint8(0)
		if h.Failure != nil {
			if h.Failure.FromBlock != from || h.Failure.ToBlock > e.result.BlockNumber || h.Failure.ToBlock-from >= e.r.limits.LogBlockRange {
				e.err = fmt.Errorf("failed to resume lp operators: failure_range=invalid")
				return false
			}
			to = h.Failure.ToBlock
			attempts = h.Failure.Attempts
		}
		calls := e.result.Metrics.Calls
		before := e.block(to, true)
		var operators []common.Address
		if before != nil {
			logs := e.logs(ethereum.FilterQuery{FromBlock: new(big.Int).SetUint64(from), ToBlock: new(big.Int).SetUint64(to), Addresses: []common.Address{e.manager}, Topics: [][]common.Hash{{event}, {ownerTopic}}})
			for _, l := range logs {
				if l.Address != e.manager || len(l.Topics) != 3 || l.Topics[0] != event || l.Topics[1] != ownerTopic || len(l.Data) != 32 || new(big.Int).SetBytes(l.Topics[2][:]).BitLen() > 160 || new(big.Int).SetBytes(l.Data).Cmp(big.NewInt(1)) > 0 || l.BlockNumber == to && l.BlockHash != before.Hash() {
					e.err = fmt.Errorf("failed to verify lp operators: approval_event=invalid")
					break
				}
				operators = append(operators, common.BytesToAddress(l.Topics[2][:]))
			}
			if e.err == nil {
				// A stable new tail does not prove the previously scanned prefix
				// belongs to that branch. Recheck it between the tail reads so a
				// reorg cannot promote mixed history into a reusable checkpoint.
				anchor := h.Through
				if anchor == nil && h.StartBlock != 0 {
					anchor = &BlockReference{Number: h.StartBlock, Hash: h.StartHash}
				}
				if anchor == nil || e.canonicalFresh(*anchor) {
					e.block(to, true)
				}
			}
		}
		if e.err != nil {
			if e.result.Metrics.Calls > calls && !errors.Is(e.err, ErrBudget) {
				h.Failure = &HistoryFailure{FromBlock: from, ToBlock: to, Attempts: attempts + 1, Reason: "acquisition_failed"}
				save()
			}
			return false
		}
		unique := make(map[common.Address]bool)
		for _, a := range h.Operators {
			unique[a] = true
		}
		for _, a := range operators {
			unique[a] = true
		}
		if len(unique) > e.r.limits.MaxLogs {
			e.err = fmt.Errorf("failed to retain lp operators: %w: operators=too_long", ErrBudget)
			return false
		}
		h.Operators = nil
		for a := range unique {
			h.Operators = append(h.Operators, a)
		}
		sort.Slice(h.Operators, func(i, j int) bool { return h.Operators[i].Hex() < h.Operators[j].Hex() })
		h.Through = &BlockReference{Number: to, Hash: before.Hash()}
		h.Failure = nil
		save()
		done = to == e.result.BlockNumber
		if !done {
			from = to + 1
		}
	}
	for _, operator := range h.Operators {
		approved := e.word(e.manager, "isApprovedForAll(address,address)", p.Owner.Big(), operator.Big())
		if e.err != nil {
			return false
		}
		if approved.Cmp(big.NewInt(1)) > 0 {
			e.err = fmt.Errorf("failed to verify lp operators: approval=invalid")
			return false
		}
		if approved.Sign() != 0 {
			p.Reason = "operator_approval_active"
			e.operatorChecks[p.Owner] = p.Reason
			return false
		}
	}
	e.operatorChecks[p.Owner] = ""
	p.Reason = ""
	return true
}
