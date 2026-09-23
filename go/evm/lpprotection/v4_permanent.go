package lpprotection

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const permanentRule = "v4-launch-factory-create-locker-v1"
const launchFactoryInitSHA256 = "2acc2c3d3f93674e48f1be9de3c7b5025d4cfdc1b7399fd3a3c03f4265088603"

func (e *evaluation) v4PermanentCustody(p *Position, fields map[string]*big.Int) {
	p.Model = permanentRule
	p.Reason = "creation_evidence_unavailable"
	if fields["manager"].Cmp(e.manager.Big()) != 0 || fields["factory"].BitLen() > 160 || fields["factory"].Sign() == 0 {
		p.Reason = "immutable_binding_mismatch"
		return
	}
	if !e.permanentAuthorityClear(p) {
		p.Reason = "authority_conflict"
		return
	}
	if e.r.creationRPC == nil || e.err != nil {
		return
	}
	factory := common.BigToAddress(fields["factory"])
	code := e.code(factory)
	f, ok := match(code, "v4LaunchFactory")
	if !ok {
		p.Reason = "factory_code_unresolved"
		return
	}
	permit2 := common.HexToAddress("0x000000000022d473030f116ddee9f6b43ac78ba3")
	if f["poolManager"].Cmp(e.result.Pool.Big()) != 0 || f["manager"].Cmp(e.manager.Big()) != 0 || f["permit2"].Cmp(permit2.Big()) != 0 || f["locker"].Cmp(p.Owner.Big()) != 0 || crypto.CreateAddress(factory, 1) != p.Owner {
		p.Reason = "immutable_binding_mismatch"
		return
	}
	hint := e.permanentCreationHint(factory, p.Owner)
	if e.err != nil || hint == (common.Hash{}) {
		return
	}
	tx := e.creationTransaction(hint)
	if tx == nil {
		return
	}
	origin, ok := launchFactoryDeployment(tx, factory, e.result.Pool, e.manager, permit2)
	if !ok {
		p.Reason = "creation_path_unresolved"
		return
	}
	receipt := e.creationReceipt(hint)
	if receipt == nil {
		return
	}
	if receipt.ContractAddress != factory || receipt.Type != tx.Type() {
		p.Reason = "creation_receipt_mismatch"
		return
	}
	birth := BlockReference{Number: receipt.BlockNumber.Uint64(), Hash: receipt.BlockHash}
	if !e.canonical(birth) {
		return
	}
	if !bytes.Equal(e.historicalCode(factory, birth.Hash), code) || !bytes.Equal(e.historicalCode(p.Owner, birth.Hash), e.code(p.Owner)) {
		p.Reason = "creation_runtime_mismatch"
		return
	}
	// Creation acquisition may have supplied more receipts. Check them before
	// publishing a proof; never substitute an empty operator history for proof.
	if e.err != nil || !e.permanentAuthorityClear(p) {
		p.Reason = "authority_conflict"
		return
	}
	proof := PermanentCustodyEvidence{
		Rule: permanentRule, Manager: e.manager, Factory: factory, Custodian: p.Owner,
		Sender: origin, FactoryTransaction: hint, CreationBlock: birth,
		ManagerCodeHash: crypto.Keccak256Hash(e.code(e.manager)),
		FactoryCodeHash: crypto.Keccak256Hash(code), CustodianCodeHash: p.CodeHash,
		InitCodeHash: crypto.Keccak256Hash(tx.Data()), FactoryNonce: 0, LockerNonce: 1,
	}
	if e.err != nil {
		return
	}
	index := -1
	for i, old := range e.evidence.data.PermanentCustodies {
		if old.Manager == proof.Manager && old.Custodian == proof.Custodian {
			index = i
			break
		}
	}
	if index < 0 {
		e.evidence.data.PermanentCustodies = append(e.evidence.data.PermanentCustodies, proof)
	} else {
		e.evidence.data.PermanentCustodies[index] = proof
	}
	if e.permanentAnchors == nil {
		e.permanentAnchors = make(map[uint64]common.Hash)
	}
	e.permanentAnchors[birth.Number] = birth.Hash
	e.permanentAnchors[e.req.Creation.BlockNumber] = e.req.Creation.BlockHash
	p.Kind, p.Reason, p.CanWeakenProtection = "permanent", "", flag(false)
}

func (e *evaluation) permanentCreationHint(factory, custodian common.Address) common.Hash {
	var hint common.Hash
	for _, h := range e.req.CreationHints {
		if h.Factory != factory {
			continue
		}
		if h.TransactionHash == (common.Hash{}) || hint != (common.Hash{}) && hint != h.TransactionHash {
			return common.Hash{}
		}
		hint = h.TransactionHash
	}
	if hint != (common.Hash{}) {
		return hint
	}
	for _, c := range e.evidence.data.PermanentCustodies {
		if c.Factory == factory && c.Custodian == custodian && c.Manager == e.manager {
			return c.FactoryTransaction
		}
	}
	if e.r.resolver != nil {
		resolved, err := e.r.resolver.ResolveCreationHint(e.ctx, e.req.ChainID, factory)
		if err != nil {
			e.err = fmt.Errorf("failed to resolve permanent custody creation: %w", err)
			return common.Hash{}
		}
		if resolved.TransactionHash != (common.Hash{}) && resolved.Factory != factory {
			e.err = fmt.Errorf("failed to resolve permanent custody creation: factory=invalid")
			return common.Hash{}
		}
		return resolved.TransactionHash
	}
	return common.Hash{}
}

func launchFactoryDeployment(tx *types.Transaction, factory, poolManager, manager, permit2 common.Address) (common.Address, bool) {
	if tx.Type() != types.DynamicFeeTxType || tx.To() != nil || tx.Nonce() != 0 || tx.Value().Sign() != 0 || tx.ChainId().Cmp(big.NewInt(8453)) != 0 {
		return common.Address{}, false
	}
	origin, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8453)), tx)
	if err != nil || crypto.CreateAddress(origin, 0) != factory {
		return common.Address{}, false
	}
	data := tx.Data()
	const length = 19996
	if len(data) != length+96 {
		return common.Address{}, false
	}
	digest := sha256.Sum256(data[:length])
	if hex.EncodeToString(digest[:]) != launchFactoryInitSHA256 || !bytes.Equal(data[length:], query("constructor()", poolManager.Big(), manager.Big(), permit2.Big())[4:]) {
		return common.Address{}, false
	}
	return origin, true
}

func (e *evaluation) finishPermanentEvidence() {
	blocks := make([]uint64, 0, len(e.permanentAnchors))
	for number := range e.permanentAnchors {
		blocks = append(blocks, number)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i] < blocks[j] })
	for _, number := range blocks {
		if !e.canonicalFresh(BlockReference{Number: number, Hash: e.permanentAnchors[number]}) {
			return
		}
	}
}
