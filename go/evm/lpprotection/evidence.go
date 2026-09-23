package lpprotection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// CreationHint identifies a candidate factory deployment transaction. Source
// services may supply hints; the reader verifies the actual transaction/code.
type CreationHint struct {
	Factory         common.Address
	TransactionHash common.Hash
}

type BlockReference struct {
	Number uint64
	Hash   common.Hash
}

type CreationEvidence struct {
	Rule                                                       string
	Factory, Custodian, Implementation                         common.Address
	FactoryTransaction                                         common.Hash
	FactoryBlock, CustodianBlock                               BlockReference
	FactoryCodeHash, CustodianCodeHash, ImplementationCodeHash common.Hash
	CreateNonce                                                uint64
	LockEvent                                                  types.Log
}

type HistoryFailure struct {
	FromBlock, ToBlock uint64
	Attempts           uint8
	Reason             string
}

type OperatorHistory struct {
	Manager, Custodian                 common.Address
	ManagerCodeHash, CustodianCodeHash common.Hash
	StartBlock                         uint64
	// StartHash is set only when a reviewed creation proves the nonzero start.
	StartHash common.Hash
	Through   *BlockReference
	Operators []common.Address
	Failure   *HistoryFailure
}

// Evidence is an immutable, reader-produced internal cache. It has no public
// complete flag or writable history fields. JSON persistence is intentionally
// restored only through RestoreEvidence, never from an API request payload.
// This cache trusts its producer/storage just as a supplied receipt cache does;
// block hashes are not a cryptographic proof that no logs were omitted.
type Evidence struct{ data evidenceData }

type evidenceData struct {
	ModelVersion       string
	ChainID            uint64
	Creations          []CreationEvidence
	Histories          []OperatorHistory
	Acquired           map[string]json.RawMessage
	PermanentCustodies []PermanentCustodyEvidence `json:",omitempty"`
	PermanentConflicts []permanentConflict        `json:",omitempty"`
}

// MarshalJSON serializes bounded evidence for trusted internal persistence.
//
// Version:
//   - 2026-09-23: Added.
func (e *Evidence) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	b, err := json.Marshal(e.data)
	if err != nil {
		return nil, fmt.Errorf("failed to encode lp evidence: %w", err)
	}
	return b, nil
}

// RestoreEvidence restores only a reader-produced snapshot from trusted storage.
// It checks shape/version; Analyze rechecks canonicality and mutable state.
// Never pass arbitrary user JSON here: local cache integrity is a caller duty.
//
// Parameters:
//   - data: JSON previously returned by Evidence.MarshalJSON.
//   - maxBytes: maximum stored evidence size, normally Limits.MaxResponseBytes.
//
// Version:
//   - 2026-09-24: Preserve validated v4 acquisition ledgers when migrating to permanent-custody evidence.
//   - 2026-09-23: Require v4 evidence with reorg-safe history checkpoints.
func RestoreEvidence(data []byte, maxBytes int) (*Evidence, error) {
	if maxBytes < 1 || len(data) > maxBytes {
		return nil, fmt.Errorf("failed to restore lp evidence: %w: bytes=out_of_range", ErrBudget)
	}
	e := &Evidence{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e.data); err != nil {
		return nil, fmt.Errorf("failed to restore lp evidence: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("failed to restore lp evidence: %w", err)
		}
		return nil, fmt.Errorf("failed to restore lp evidence: trailing_data=invalid")
	}
	if e.data.ModelVersion != ModelVersion && e.data.ModelVersion != previousModelVersion || e.data.ChainID != 8453 {
		return nil, fmt.Errorf("failed to restore lp evidence: identity=invalid")
	}
	if e.data.ModelVersion == previousModelVersion {
		// The old schema cannot contain new proof fields, even explicit nulls.
		// Its existing creation/history validator below must still succeed.
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return nil, fmt.Errorf("failed to migrate lp evidence: %w", err)
		}
		for key := range fields {
			if key != "ModelVersion" && key != "ChainID" && key != "Creations" && key != "Histories" && key != "Acquired" {
				return nil, fmt.Errorf("failed to migrate lp evidence: schema=invalid")
			}
		}
	}
	seen := make(map[string]bool)
	for _, h := range e.data.Histories {
		key := h.Manager.Hex() + h.Custodian.Hex()
		if seen[key] || h.Manager == (common.Address{}) || h.Custodian == (common.Address{}) || h.ManagerCodeHash == (common.Hash{}) || h.CustodianCodeHash == (common.Hash{}) || (h.StartBlock == 0) != (h.StartHash == (common.Hash{})) {
			return nil, fmt.Errorf("failed to restore lp evidence: history=invalid")
		}
		seen[key] = true
		next := h.StartBlock
		if h.Through == nil && len(h.Operators) != 0 {
			return nil, fmt.Errorf("failed to restore lp evidence: checkpoint=empty")
		}
		if h.Through != nil {
			if h.Through.Hash == (common.Hash{}) || h.Through.Number < h.StartBlock || h.Through.Number == ^uint64(0) {
				return nil, fmt.Errorf("failed to restore lp evidence: checkpoint=invalid")
			}
			next = h.Through.Number + 1
		}
		if h.Failure != nil && (h.Failure.FromBlock != next || h.Failure.ToBlock < next || h.Failure.Attempts == 0 || h.Failure.Attempts > 4 || h.Failure.Reason != "acquisition_failed") {
			return nil, fmt.Errorf("failed to restore lp evidence: failure_range=invalid")
		}
		unique := make(map[common.Address]bool)
		for _, a := range h.Operators {
			if unique[a] {
				return nil, fmt.Errorf("failed to restore lp evidence: operators=invalid")
			}
			unique[a] = true
		}
	}
	seen = make(map[string]bool)
	for _, c := range e.data.Creations {
		key := c.Custodian.Hex()
		if seen[key] || c.Rule != creationRule || c.Factory == (common.Address{}) || c.Custodian == (common.Address{}) || c.Implementation == (common.Address{}) || c.FactoryTransaction == (common.Hash{}) || c.FactoryBlock.Hash == (common.Hash{}) || c.CustodianBlock.Hash == (common.Hash{}) || c.FactoryBlock.Number == 0 || c.CustodianBlock.Number < c.FactoryBlock.Number || c.CreateNonce == 0 || c.LockEvent.Address != c.Factory || c.LockEvent.BlockHash != c.CustodianBlock.Hash || c.LockEvent.BlockNumber != c.CustodianBlock.Number {
			return nil, fmt.Errorf("failed to restore lp evidence: creation=invalid")
		}
		seen[key] = true
	}
	for _, h := range e.data.Histories {
		if h.StartBlock == 0 {
			continue
		}
		bound := false
		for _, c := range e.data.Creations {
			if c.Custodian == h.Custodian && c.CustodianBlock.Number == h.StartBlock && c.CustodianBlock.Hash == h.StartHash && c.CustodianCodeHash == h.CustodianCodeHash {
				bound = true
				break
			}
		}
		if !bound {
			return nil, fmt.Errorf("failed to restore lp evidence: creation_binding=invalid")
		}
	}
	if e.data.Acquired == nil {
		e.data.Acquired = make(map[string]json.RawMessage)
	}
	if err := validatePermanentEvidence(e.data); err != nil {
		return nil, fmt.Errorf("failed to restore lp evidence: %w", err)
	}
	e.data.ModelVersion = ModelVersion
	return e, nil
}

// Histories returns detached copies of acquisition progress, including failures.
// Callers can schedule backoff without changing the reader's evidence.
//
// Version:
//   - 2026-09-23: Added.
func (e *Evidence) Histories() []OperatorHistory {
	if e == nil {
		return nil
	}
	result := append([]OperatorHistory(nil), e.data.Histories...)
	for i := range result {
		result[i].Operators = append([]common.Address(nil), result[i].Operators...)
		if p := result[i].Through; p != nil {
			v := *p
			result[i].Through = &v
		}
		if p := result[i].Failure; p != nil {
			v := *p
			result[i].Failure = &v
		}
	}
	return result
}

// Creations returns detached copies of verified creation bindings.
//
// Version:
//   - 2026-09-23: Added.
func (e *Evidence) Creations() []CreationEvidence {
	if e == nil {
		return nil
	}
	result := append([]CreationEvidence(nil), e.data.Creations...)
	for i := range result {
		result[i].LockEvent.Topics = append([]common.Hash(nil), result[i].LockEvent.Topics...)
		result[i].LockEvent.Data = bytes.Clone(result[i].LockEvent.Data)
	}
	return result
}

func (e *evaluation) cacheEvidence(key string, value any) {
	if e.err != nil {
		return
	}
	b, err := json.Marshal(value)
	if err != nil {
		e.err = fmt.Errorf("failed to retain lp evidence: %w", err)
		return
	}
	old, exists := e.evidence.data.Acquired[key]
	e.evidence.data.Acquired[key] = b
	encoded, err := e.evidence.MarshalJSON()
	if err != nil {
		e.err = err
		return
	}
	if len(encoded) > e.r.limits.MaxResponseBytes {
		if exists {
			e.evidence.data.Acquired[key] = old
		} else {
			delete(e.evidence.data.Acquired, key)
		}
		e.err = fmt.Errorf("failed to retain lp evidence: %w: bytes=too_long", ErrBudget)
	}
}

func (e *evaluation) cachedEvidence(key string, value any) bool {
	b, ok := e.evidence.data.Acquired[key]
	if !ok {
		return false
	}
	if err := json.Unmarshal(b, value); err != nil {
		e.err = fmt.Errorf("failed to reuse lp evidence: %w", err)
	}
	e.result.Metrics.CacheHits++
	return true
}
