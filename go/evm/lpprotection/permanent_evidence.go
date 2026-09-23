package lpprotection

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// PermanentCustodyEvidence binds a reviewed constructor and runtime to a first
// deployment. It is reader-produced evidence, not a user-supplied verdict.
type PermanentCustodyEvidence struct {
	Rule                                                              string
	Manager, Factory, Custodian, Sender                               common.Address
	FactoryTransaction                                                common.Hash
	CreationBlock                                                     BlockReference
	ManagerCodeHash, FactoryCodeHash, CustodianCodeHash, InitCodeHash common.Hash
	FactoryNonce, LockerNonce                                         uint64
}

type permanentConflict struct {
	Manager, Custodian common.Address
	Block              BlockReference
}

// PermanentCustodies returns detached creation and runtime bindings for permanent custody.
// Reuse must still go through Analyze to recheck canonicality, runtime and NFT state.
//
// Version:
//   - 2026-09-24: Added.
func (e *Evidence) PermanentCustodies() []PermanentCustodyEvidence {
	if e == nil {
		return nil
	}
	return append([]PermanentCustodyEvidence(nil), e.data.PermanentCustodies...)
}

func validatePermanentEvidence(data evidenceData) error {
	seen := make(map[[2]common.Address]bool)
	for _, c := range data.PermanentCustodies {
		key := [2]common.Address{c.Manager, c.Custodian}
		if seen[key] || c.Rule != permanentRule || c.Manager == (common.Address{}) || c.Factory == (common.Address{}) || c.Custodian == (common.Address{}) || c.Sender == (common.Address{}) || c.FactoryTransaction == (common.Hash{}) || c.CreationBlock.Number == 0 || c.CreationBlock.Hash == (common.Hash{}) || c.ManagerCodeHash == (common.Hash{}) || c.FactoryCodeHash == (common.Hash{}) || c.CustodianCodeHash == (common.Hash{}) || c.InitCodeHash == (common.Hash{}) || c.FactoryNonce != 0 || c.LockerNonce != 1 || crypto.CreateAddress(c.Sender, 0) != c.Factory || crypto.CreateAddress(c.Factory, 1) != c.Custodian {
			return fmt.Errorf("failed to validate permanent custody evidence: binding=invalid")
		}
		seen[key] = true
	}
	conflicts := make(map[permanentConflict]bool)
	for _, c := range data.PermanentConflicts {
		if conflicts[c] || c.Manager == (common.Address{}) || c.Custodian == (common.Address{}) || c.Block.Number == 0 || c.Block.Hash == (common.Hash{}) {
			return fmt.Errorf("failed to validate permanent custody evidence: conflict=invalid")
		}
		conflicts[c] = true
	}
	return nil
}

func (e *evaluation) recordPermanentConflict(owner common.Address, block BlockReference) {
	c := permanentConflict{Manager: e.manager, Custodian: owner, Block: block}
	for _, old := range e.evidence.data.PermanentConflicts {
		if old == c {
			return
		}
	}
	e.evidence.data.PermanentConflicts = append(e.evidence.data.PermanentConflicts, c)
}

func (e *evaluation) permanentConflictsClear(owner common.Address) bool {
	clear := true
	kept := make([]permanentConflict, 0, len(e.evidence.data.PermanentConflicts))
	for _, c := range e.evidence.data.PermanentConflicts {
		if c.Manager != e.manager || c.Custodian != owner {
			kept = append(kept, c)
			continue
		}
		// An older requested observation cannot erase a later conflict.
		if c.Block.Number > e.result.BlockNumber {
			kept = append(kept, c)
			clear = false
			continue
		}
		h := e.block(c.Block.Number, false)
		if e.err != nil || h == nil {
			return false // Preserve all conflicts if validation could not complete.
		}
		if h.Hash() == c.Block.Hash {
			kept = append(kept, c)
			clear = false
		} else {
			// Drop only a conflict whose own anchor was orphaned. Other
			// canonical conflicts survive the resulting global invalidation.
			e.invalidateEvidence()
			// Preserve unvisited entries as well; remove this exact anchor only.
			kept = make([]permanentConflict, 0, len(e.evidence.data.PermanentConflicts))
			for _, rest := range e.evidence.data.PermanentConflicts {
				if rest != c {
					kept = append(kept, rest)
				}
			}
			e.evidence.data.PermanentConflicts = kept
			return false
		}
	}
	e.evidence.data.PermanentConflicts = kept
	return clear
}
