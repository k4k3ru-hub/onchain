package lpprotection

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const creationRule = "cl-factory-create3-clone-v1"

// clCreationStart recognizes one reviewed code path, not a locker address list.
// CreateX's fixed CREATE2 proxy cannot delete itself/reset its CREATE nonce.
// Its nonce-1 child therefore has a first incarnation bound to this init code.
// The matched factory has neither SELFDESTRUCT nor arbitrary DELEGATECALL and
// creates registered lockers using CREATE. Its nonce range binds their birth.
func (e *evaluation) clCreationStart(p *Position, factory, implementation common.Address) {
	if e.r.creationRPC == nil || e.err != nil {
		return
	}
	var hint common.Hash
	for _, h := range e.req.CreationHints {
		if h.Factory == factory {
			if hint != (common.Hash{}) && hint != h.TransactionHash {
				return
			}
			hint = h.TransactionHash
		}
	}
	var candidate *types.Log
	consider := func(l *types.Log) bool {
		if l == nil || l.Address != factory || len(l.Topics) < 3 || l.Topics[0] != crypto.Keccak256Hash([]byte("LockCreated(address,address,uint256,uint32,address,uint16,uint16)")) || l.Topics[2] != common.BytesToHash(p.Owner[:]) {
			return true
		}
		if !validLockEvent(l, p.ID) {
			return false
		}
		if candidate != nil && (candidate.TxHash != l.TxHash || candidate.Index != l.Index || candidate.BlockHash != l.BlockHash) {
			return false
		}
		candidate = l
		return true
	}
	for _, c := range e.evidence.data.Creations {
		if c.Custodian == p.Owner && c.Factory == factory {
			if hint == (common.Hash{}) {
				hint = c.FactoryTransaction
			}
			l := c.LockEvent
			if !consider(&l) {
				return
			}
		}
	}
	for _, receipts := range []map[common.Hash]*types.Receipt{e.req.Receipts, e.receipts} {
		for _, r := range receipts {
			if r != nil {
				for _, l := range r.Logs {
					if !consider(l) {
						return
					}
				}
			}
		}
	}
	if candidate == nil || candidate.BlockNumber == 0 || candidate.BlockNumber > e.result.BlockNumber {
		return
	}
	if hint == (common.Hash{}) && e.r.resolver != nil {
		resolved, err := e.r.resolver.ResolveCreationHint(e.ctx, e.req.ChainID, factory)
		if err != nil {
			e.err = fmt.Errorf("failed to resolve lp creation hint: %w", err)
			return
		}
		if resolved.TransactionHash != (common.Hash{}) && resolved.Factory != factory {
			e.err = fmt.Errorf("failed to resolve lp creation hint: factory=invalid")
			return
		}
		hint = resolved.TransactionHash
	}
	if hint == (common.Hash{}) {
		return
	}
	receipt := e.creationReceipt(candidate.TxHash)
	if receipt == nil || e.receipt(*candidate) == nil {
		return
	}
	birth := BlockReference{candidate.BlockNumber, candidate.BlockHash}
	if !e.canonical(birth) {
		return
	}
	factoryReceipt := e.creationReceipt(hint)
	if factoryReceipt == nil {
		return
	}
	factoryBlock := BlockReference{factoryReceipt.BlockNumber.Uint64(), factoryReceipt.BlockHash}
	// Same-block factory/locker deployment is outside this initial nonce rule.
	if factoryBlock.Number >= birth.Number || !e.canonical(factoryBlock) {
		return
	}
	tx := e.creationTransaction(hint)
	if tx == nil {
		return
	}
	currentFactory := e.code(factory)
	fields, ok := match(currentFactory, "clFactory")
	if !ok {
		return
	}
	deployer, proxy, salt, ok := factoryDeployment(tx, factory, fields)
	if !ok || !creationEvents(factoryReceipt, deployer, proxy, factory, salt) {
		return
	}
	expectedCreateX := common.HexToHash("0xbd8a7ea8cfca7b4e5f5041d7d4b17bc317c5ce42cfbc42066a00cf26b43eb53f")
	parent := e.headers[factoryBlock.Number].ParentHash
	if crypto.Keccak256Hash(e.historicalCode(deployer, factoryBlock.Hash)) != expectedCreateX || crypto.Keccak256Hash(e.historicalCode(deployer, parent)) != expectedCreateX || !bytes.Equal(e.historicalCode(proxy, factoryBlock.Hash), common.FromHex("0x363d3d37363d34f0")) {
		return
	}
	if !bytes.Equal(e.historicalCode(factory, factoryBlock.Hash), currentFactory) || !bytes.Equal(e.historicalCode(factory, birth.Hash), currentFactory) {
		return
	}
	clone := e.historicalCode(p.Owner, birth.Hash)
	if !bytes.Equal(clone, e.code(p.Owner)) || crypto.Keccak256Hash(e.historicalCode(implementation, birth.Hash)) != crypto.Keccak256Hash(e.code(implementation)) {
		return
	}
	before := e.historicalNonce(factory, e.headers[birth.Number].ParentHash)
	after := e.historicalNonce(factory, birth.Hash)
	if e.err != nil || before == 0 || before >= after || after-before > 1024 {
		return
	}
	var nonce uint64
	for n := before; n < after; n++ {
		if crypto.CreateAddress(factory, n) == p.Owner {
			nonce = n
			break
		}
	}
	if nonce == 0 {
		return
	}
	// These historical blocks are rechecked before publishing a reusable proof.
	if e.block(factoryBlock.Number, true) == nil || e.block(birth.Number, true) == nil {
		return
	}
	proof := CreationEvidence{Rule: creationRule, Factory: factory, Custodian: p.Owner, Implementation: implementation, FactoryTransaction: hint, FactoryBlock: factoryBlock, CustodianBlock: birth, FactoryCodeHash: crypto.Keccak256Hash(currentFactory), CustodianCodeHash: p.CodeHash, ImplementationCodeHash: crypto.Keccak256Hash(e.code(implementation)), CreateNonce: nonce, LockEvent: *candidate}
	replaced := false
	for i, c := range e.evidence.data.Creations {
		if c.Custodian == p.Owner {
			e.evidence.data.Creations[i] = proof
			replaced = true
			break
		}
	}
	if !replaced {
		e.evidence.data.Creations = append(e.evidence.data.Creations, proof)
	}
	e.starts[p.Owner] = birth
}

func validLockEvent(l *types.Log, id *big.Int) bool {
	if l.Removed || l.BlockHash == (common.Hash{}) || l.TxHash == (common.Hash{}) || len(l.Topics) != 3 || len(l.Data) != 160 || new(big.Int).SetBytes(l.Topics[1][:]).BitLen() > 160 || new(big.Int).SetBytes(l.Data[:32]).Cmp(id) != 0 {
		return false
	}
	return new(big.Int).SetBytes(l.Data[32:64]).BitLen() <= 32 && new(big.Int).SetBytes(l.Data[64:96]).BitLen() <= 160 && new(big.Int).SetBytes(l.Data[96:128]).BitLen() <= 16 && new(big.Int).SetBytes(l.Data[128:]).BitLen() <= 16
}

func factoryDeployment(tx *types.Transaction, factory common.Address, fields map[string]*big.Int) (common.Address, common.Address, common.Hash, bool) {
	fail := func() (common.Address, common.Address, common.Hash, bool) {
		return common.Address{}, common.Address{}, common.Hash{}, false
	}
	if tx.To() == nil || tx.Value().Sign() != 0 || tx.ChainId().Cmp(big.NewInt(8453)) != 0 {
		return fail()
	}
	origin, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8453)), tx)
	if err != nil {
		return fail()
	}
	d := tx.Data()
	const initLength = 12988 + 160
	const padded = (initLength + 31) / 32 * 32
	if len(d) != 100+padded || !bytes.Equal(d[:4], query("deployCreate3(bytes32,bytes)")[:4]) || new(big.Int).SetBytes(d[36:68]).Cmp(big.NewInt(64)) != 0 || new(big.Int).SetBytes(d[68:100]).Cmp(big.NewInt(initLength)) != 0 {
		return fail()
	}
	// Limit _guard to the sender-bound, non-chain-specific salt branch verified
	// by the archived replay. Other valid CreateX methods/salts stay unresolved.
	if !bytes.Equal(d[4:24], origin[:]) || d[24] != 0 {
		return fail()
	}
	for _, b := range d[100+initLength:] {
		if b != 0 {
			return fail()
		}
	}
	init := d[100 : 100+initLength]
	digest := sha256.Sum256(init[:12988])
	if hex.EncodeToString(digest[:]) != "bbe406b3ec56ea8796f0ef27c923c1b79e907914098bf8b463cdf7edfca06c61" {
		return fail()
	}
	args := init[12988:]
	for i, name := range []string{"owner", "poolLauncher", "implementation", "manager", "voter"} {
		value := new(big.Int).SetBytes(args[i*32 : (i+1)*32])
		if value.BitLen() > 160 || name != "owner" && (fields[name] == nil || fields[name].Cmp(value) != 0) {
			return fail()
		}
	}
	salt := crypto.Keccak256Hash(common.LeftPadBytes(origin[:], 32), d[4:36])
	proxy := crypto.CreateAddress2(*tx.To(), salt, crypto.Keccak256(common.FromHex("0x67363d3d37363d34f03d5260086018f3")))
	if crypto.CreateAddress(proxy, 1) != factory {
		return fail()
	}
	return *tx.To(), proxy, salt, true
}

func creationEvents(r *types.Receipt, deployer, proxy, factory common.Address, salt common.Hash) bool {
	proxyTopic := crypto.Keccak256Hash([]byte("Create3ProxyContractCreation(address,bytes32)"))
	factoryTopic := crypto.Keccak256Hash([]byte("ContractCreation(address)"))
	proxies, factories := 0, 0
	var pi, fi uint
	for _, l := range r.Logs {
		if l.Address != deployer || len(l.Topics) == 0 {
			continue
		}
		if l.Topics[0] == proxyTopic {
			if len(l.Data) != 0 || len(l.Topics) != 3 || l.Topics[1] != common.BytesToHash(proxy[:]) || l.Topics[2] != salt {
				return false
			}
			proxies++
			pi = l.Index
		}
		if l.Topics[0] == factoryTopic {
			if len(l.Data) != 0 || len(l.Topics) != 2 || l.Topics[1] != common.BytesToHash(factory[:]) {
				return false
			}
			factories++
			fi = l.Index
		}
	}
	return proxies == 1 && factories == 1 && pi < fi
}
