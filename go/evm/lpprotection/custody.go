package lpprotection

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type codeTemplate struct {
	length int
	digest string
	fields map[string][]int
}

func match(code []byte, model string) (map[string]*big.Int, bool) {
	t := template(model)
	if t.length == 0 || len(code) != t.length {
		return nil, false
	}
	normalized := bytes.Clone(code)
	fields := make(map[string]*big.Int)
	for name, sites := range t.fields {
		for _, offset := range sites {
			if offset < 0 || offset+32 > len(code) {
				return nil, false
			}
			value := new(big.Int).SetBytes(code[offset : offset+32])
			if previous, ok := fields[name]; ok && previous.Cmp(value) != 0 {
				return nil, false
			}
			fields[name] = value
			clear(normalized[offset : offset+32])
		}
	}
	sum := sha256.Sum256(normalized)
	return fields, hex.EncodeToString(sum[:]) == t.digest
}

func (e *evaluation) custody(p *Position) {
	p.Owner = e.address(e.manager, "ownerOf(uint256)", p.ID)
	p.Approved = e.address(e.manager, "getApproved(uint256)", p.ID)
	code := e.code(p.Owner)
	if e.err != nil {
		return
	}
	p.CodeHash = crypto.Keccak256Hash(code)
	if ordinary(p.Owner, code) {
		data := query("decreaseLiquidity((uint256,uint128,uint256,uint256,uint256))", p.ID, p.Liquidity, new(big.Int), new(big.Int), new(big.Int).SetInt64(e.result.BlockTime.Unix()+3600))
		response := e.call(p.Owner, e.manager, data)
		if e.err == nil && len(response) != 64 {
			e.err = fmt.Errorf("failed to verify lp withdrawal: response=invalid")
		}
		if e.err == nil {
			p.Kind = "withdrawable"
			p.Model = "direct-eoa-v1"
			p.CanWeakenProtection = flag(false)
		}
		return
	}
	// A nonzero NFT approval is not evidence that a contract spender can act.
	// Until that path is reviewed, neither protected nor withdrawable is claimed.
	if p.Approved != (common.Address{}) {
		p.Reason = "approval_path_unresolved"
		return
	}
	if fields, ok := match(code, "vault"); ok {
		e.vault(p, fields)
		return
	}
	if len(code) == 45 && bytes.Equal(code[:10], common.FromHex("0x363d3d373d3d3d363d73")) && bytes.Equal(code[30:], common.FromHex("0x5af43d82803e903d91602b57fd5bf3")) {
		implementation := common.BytesToAddress(code[10:30])
		if fields, ok := match(e.code(implementation), "clLocker"); ok {
			e.clLocker(p, implementation, fields)
			return
		}
	}
	p.Reason = "unsupported_custody_code"
}

func (e *evaluation) vault(p *Position, fields map[string]*big.Int) {
	p.Model = "reviewed-multivault-v1"
	// Key ownership introduces an external authorization contract. The first
	// rule deliberately covers only the immutable initial EOA beneficiary.
	if fields["beneficiary"].BitLen() > 160 || e.word(p.Owner, "vaultKeyId()").Sign() != 0 {
		p.Reason = "vault_key_authority_unresolved"
		return
	}
	beneficiary := common.BigToAddress(fields["beneficiary"])
	if !ordinary(beneficiary, e.code(beneficiary)) {
		p.Reason = "beneficiary_authority_unresolved"
		return
	}
	until := e.word(p.Owner, "unlockTimestamp()")
	if !until.IsInt64() || until.Sign() <= 0 {
		p.Reason = "unlock_time_unresolved"
		return
	}
	if until.Int64() <= e.result.BlockTime.Unix() {
		// Exercise the actual beneficiary entry point, including factory callback.
		response := e.call(beneficiary, p.Owner, query("partialNonFungibleTokenUnlock(address,uint256)", e.manager.Big(), p.ID))
		if e.err == nil && len(response) != 0 {
			e.err = fmt.Errorf("failed to verify vault withdrawal: response=invalid")
		}
		if e.err == nil {
			p.Kind = "withdrawable"
			p.CanWeakenProtection = flag(false)
		}
		return
	}
	// Reviewed runtime alone cannot exclude approvals made by constructor code.
	if e.word(p.Owner, "isUnlocked()").Sign() != 0 {
		p.Reason = "vault_state_unresolved"
		return
	}
	if e.err != nil || !e.operatorsClear(p) {
		return
	}
	t := time.Unix(until.Int64(), 0).UTC()
	p.UnlockAt = &t
	p.Kind = "locked"
	p.CanWeakenProtection = flag(false)
}

func (e *evaluation) clLocker(p *Position, implementation common.Address, fields map[string]*big.Int) {
	p.Model = "reviewed-cl-locker-v1"
	for _, name := range []string{"manager", "factory", "voter"} {
		if fields[name].BitLen() > 160 {
			p.Reason = "immutable_binding_mismatch"
			return
		}
	}
	factory := common.BigToAddress(fields["factory"])
	voter := common.BigToAddress(fields["voter"])
	// Only the reviewed Base Slipstream infrastructure is in the initial scope.
	if common.BigToAddress(fields["manager"]) != e.manager || voter != common.HexToAddress("0x16613524e02ad97eDfeF371bC883F2F5d6C480A5") || fields["poolType"].Cmp(big.NewInt(1)) != 0 || fields["root"].Sign() != 0 {
		p.Reason = "immutable_binding_mismatch"
		return
	}
	f, ok := match(e.code(factory), "clFactory")
	if !ok || f["manager"].Cmp(e.manager.Big()) != 0 || f["poolFactory"].Cmp(e.factory.Big()) != 0 || f["implementation"].Cmp(implementation.Big()) != 0 || f["voter"].Cmp(voter.Big()) != 0 || f["poolType"].Cmp(big.NewInt(1)) != 0 {
		p.Reason = "factory_code_unresolved"
		return
	}
	if e.address(p.Owner, "pool()") != e.result.Pool || e.word(p.Owner, "lp()").Cmp(p.ID) != 0 || e.word(factory, "instances(address)", p.Owner.Big()).Cmp(big.NewInt(1)) != 0 {
		p.Reason = "custody_binding_mismatch"
		return
	}
	owner := e.address(p.Owner, "owner()")
	until := e.word(p.Owner, "lockedUntil()")
	if !until.IsUint64() || until.BitLen() > 32 || until.Sign() == 0 || e.word(p.Owner, "staked()").Sign() != 0 {
		p.Reason = "locker_state_unresolved"
		return
	}
	if until.Int64() <= e.result.BlockTime.Unix() {
		if !ordinary(owner, e.code(owner)) {
			p.Reason = "beneficiary_authority_unresolved"
			return
		}
		response := e.call(owner, factory, query("unlock(address,address)", p.Owner.Big(), owner.Big()))
		if e.err == nil && len(response) != 0 {
			e.err = fmt.Errorf("failed to verify locker withdrawal: response=invalid")
		}
		if e.err == nil {
			p.Kind = "withdrawable"
			p.CanWeakenProtection = flag(false)
		}
		return
	}
	// A configured migration destination or gauge introduces code which this
	// rule has not reviewed. Do not label those positions protected or free.
	if e.address(factory, "newLockerFactory()") != (common.Address{}) {
		p.Reason = "migration_path_unresolved"
		return
	}
	if e.address(p.Owner, "gauge()") != (common.Address{}) || e.address(voter, "gauges(address)", e.result.Pool.Big()) != (common.Address{}) {
		p.Reason = "gauge_path_unresolved"
		return
	}
	admin := e.address(factory, "owner()")
	if e.err != nil {
		return
	}
	if admin != (common.Address{}) {
		p.CanWeakenProtection = flag(true)
	}
	e.clCreationStart(p, factory, implementation)
	if !e.operatorsClear(p) {
		p.CanWeakenProtection = nil
		return
	}
	// Even with a zero factory owner, voter governance has not been exhaustively
	// analyzed. Do not turn the absence of this one admin into a false verdict.
	t := time.Unix(until.Int64(), 0).UTC()
	p.UnlockAt = &t
	p.Kind = "locked"
}
