package lpprotection

import (
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func (e *evaluation) permanentAuthorityClear(p *Position) bool {
	if p.Approved != (common.Address{}) {
		e.recordPermanentConflict(p.Owner, BlockReference{Number: e.result.BlockNumber, Hash: e.result.BlockHash})
	}
	available := e.availableReceipts()
	ordered := make([]common.Hash, 0, len(available))
	for hash := range available {
		ordered = append(ordered, hash)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Hex() < ordered[j].Hex() })
	count := 0
	for _, hash := range ordered {
		r := available[hash]
		if r == nil {
			continue
		}
		for _, l := range r.Logs {
			if e.err != nil {
				return false
			}
			conflict, valid := permanentAuthorityEvent(l, e.manager, p.Owner)
			if !valid {
				e.err = fmt.Errorf("failed to inspect permanent custody authority: event=invalid")
				return false
			}
			if !conflict {
				continue
			}
			if r.TxHash != hash || l.TxHash != hash || l.BlockNumber == 0 || l.BlockNumber > e.result.BlockNumber {
				e.err = fmt.Errorf("failed to inspect permanent custody authority: identity=invalid")
				return false
			}
			count++
			if count > e.r.limits.MaxLogs {
				e.err = fmt.Errorf("failed to inspect permanent custody authority: %w: logs=too_long", ErrBudget)
				return false
			}
			block := BlockReference{Number: l.BlockNumber, Hash: l.BlockHash}
			if e.receipt(*l) == nil || !e.canonical(block) {
				return false
			}
			e.recordPermanentConflict(p.Owner, block)
		}
	}
	return e.err == nil && e.permanentConflictsClear(p.Owner)
}

func permanentAuthorityEvent(l *types.Log, manager, owner common.Address) (conflict, valid bool) {
	if l == nil || l.Address != manager || len(l.Topics) < 2 || l.Topics[1] != common.BytesToHash(owner[:]) {
		return false, true
	}
	switch l.Topics[0] {
	case crypto.Keccak256Hash([]byte("Approval(address,address,uint256)")):
		if len(l.Topics) != 4 || len(l.Data) != 0 || l.Topics[2].Big().BitLen() > 160 {
			return false, false
		}
		return l.Topics[2] != (common.Hash{}), true
	case crypto.Keccak256Hash([]byte("ApprovalForAll(address,address,bool)")):
		if len(l.Topics) != 3 || len(l.Data) != 32 || l.Topics[2].Big().BitLen() > 160 || common.BytesToHash(l.Data).Big().BitLen() > 1 {
			return false, false
		}
		return l.Data[31] == 1, true
	case crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)")):
		if len(l.Topics) != 4 || len(l.Data) != 0 || l.Topics[2].Big().BitLen() > 160 {
			return false, false
		}
		return true, true
	default:
		return false, true
	}
}
