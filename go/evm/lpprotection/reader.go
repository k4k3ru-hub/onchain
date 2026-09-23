package lpprotection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

// NewReader composes bounded LP analysis. No RPC runs during construction.
// The caller owns retries; every retry must share the outer acquisition budget.
//
// Version:
//   - 2026-09-23: Added.
func NewReader(rpc RPC, limits Limits) (*Reader, error) {
	if rpc == nil {
		return nil, fmt.Errorf("failed to create lp protection reader: rpc=null")
	}
	if limits.Timeout <= 0 || limits.MaxCalls < 1 || limits.MaxReceipts < 1 || limits.LogBlockRange < 1 || limits.MaxLogs < 1 || limits.MaxPositions < 1 || limits.MaxResponseBytes < 32 {
		return nil, fmt.Errorf("failed to create lp protection reader: limits=out_of_range")
	}
	return &Reader{rpc: rpc, limits: limits}, nil
}

// NewReaderWithCreationRPC composes the optional reviewed creation verifier.
// The dependencies must target the same chain and make one attempt per call.
//
// Version:
//   - 2026-09-23: Added.
func NewReaderWithCreationRPC(rpc RPC, creationRPC CreationRPC, limits Limits) (*Reader, error) {
	if creationRPC == nil {
		return nil, fmt.Errorf("failed to create lp protection reader: creation_rpc=null")
	}
	r, err := NewReader(rpc, limits)
	if err != nil {
		return nil, fmt.Errorf("failed to compose lp creation reader: %w", err)
	}
	r.creationRPC = creationRPC
	return r, nil
}

// Analyze discovers custody from a pool creation event and a reusable complete
// principal snapshot. It never recaptures full ticks, polls, retries, or submits
// transactions. Unknown custody returns no observation; acquisition failures
// return a wrapped error and metrics without publishing partial percentages.
//
// Version:
//   - 2026-09-23: Recheck history prefixes, invalidate reorg evidence and reacquire moved receipts within budget.
func (r *Reader) Analyze(ctx context.Context, req Request) (result Result, err error) {
	s := req.Principal.Snapshot
	result = Result{ModelVersion: ModelVersion, Pool: req.Principal.Pool, BlockNumber: s.BlockNumber, BlockHash: s.BlockHash, BlockTime: s.BlockTime, Metrics: Metrics{Methods: make(map[string]int)}}
	if r == nil || ctx == nil {
		return result, fmt.Errorf("failed to analyze lp protection: dependency=null")
	}
	if req.ChainID != 8453 || req.Protocol != clliquidity.V3 && req.Protocol != clliquidity.Slipstream {
		result.Reason = "unsupported_deployment"
		return result, nil
	}
	if result.Pool == (common.Address{}) || s.BlockNumber == 0 || s.BlockHash == (common.Hash{}) || s.BlockTime.Unix() <= 0 || req.Creation.Removed || req.Creation.BlockNumber > s.BlockNumber || req.Creation.BlockHash == (common.Hash{}) || req.Creation.TxHash == (common.Hash{}) {
		return result, fmt.Errorf("failed to analyze lp protection: identity=invalid")
	}
	if len(req.PositionIDs) > r.limits.MaxPositions || len(req.MintLogs) > r.limits.MaxLogs || len(req.CreationHints) > r.limits.MaxPositions {
		result.Reason = "acquisition_budget_exceeded"
		return result, fmt.Errorf("failed to analyze lp protection: %w", ErrBudget)
	}
	if _, err := clliquidity.Calculate(s.State); err != nil {
		return result, fmt.Errorf("failed to analyze lp protection: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.limits.Timeout)
	defer cancel()
	evidence := &Evidence{data: evidenceData{ModelVersion: ModelVersion, ChainID: req.ChainID, Acquired: make(map[string]json.RawMessage)}}
	if req.Evidence != nil {
		b, encodeErr := req.Evidence.MarshalJSON()
		if encodeErr != nil {
			return result, fmt.Errorf("failed to analyze lp protection: %w", encodeErr)
		}
		evidence, err = RestoreEvidence(b, r.limits.MaxResponseBytes)
		if err != nil {
			return result, fmt.Errorf("failed to analyze lp protection: %w", err)
		}
	}
	e := &evaluation{r: r, ctx: ctx, req: req, result: &result, calls: make(map[string][]byte), codes: make(map[common.Address][]byte), receipts: make(map[common.Hash]*types.Receipt), operatorChecks: make(map[common.Address]string), evidence: evidence, headers: make(map[uint64]*types.Header), starts: make(map[common.Address]BlockReference)}
	defer func() {
		b, evidenceErr := evidence.MarshalJSON()
		if evidenceErr != nil {
			e.err = errors.Join(e.err, evidenceErr)
		} else if len(b) > r.limits.MaxResponseBytes {
			e.err = errors.Join(e.err, fmt.Errorf("failed to retain lp evidence: %w: bytes=too_long", ErrBudget))
		} else {
			result.Evidence = evidence
		}
		for address, code := range e.codes {
			result.Contracts = append(result.Contracts, ContractEvidence{Address: address, CodeHash: crypto.Keccak256Hash(code)})
		}
		sort.Slice(result.Contracts, func(i, j int) bool { return result.Contracts[i].Address.Hex() < result.Contracts[j].Address.Hex() })
		if e.err != nil {
			result.Observation = nil
			result.Reason = "acquisition_failed"
			if errors.Is(e.err, ErrBudget) || errors.Is(e.err, context.DeadlineExceeded) {
				result.Reason = "acquisition_budget_exceeded"
			}
			if errors.Is(e.err, ErrHistoryAbandoned) {
				result.Reason = "operator_history_abandoned"
			}
			err = fmt.Errorf("failed to analyze lp protection: %w", e.err)
		}
	}()
	if !e.identity() {
		return result, nil
	}
	e.header()
	if e.err != nil {
		return result, nil
	}
	ids := make(map[string]*big.Int)
	defer func() {
		for _, id := range ids {
			result.PositionIDs = append(result.PositionIDs, new(big.Int).Set(id))
		}
		sort.Slice(result.PositionIDs, func(i, j int) bool { return result.PositionIDs[i].Cmp(result.PositionIDs[j]) < 0 })
	}()
	for _, id := range req.PositionIDs {
		if id == nil || id.Sign() <= 0 || id.BitLen() > 256 {
			e.err = fmt.Errorf("failed to inspect lp positions: id=invalid")
			return result, nil
		}
		ids[id.String()] = new(big.Int).Set(id)
	}
	e.discoverReceipts(ids)
	e.discover(req.MintLogs, ids)
	positions := e.positions(ids)
	if e.err != nil {
		return result, nil
	}
	if !covered(s.State, positions) {
		mint := crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)"))
		for from := req.Creation.BlockNumber; from <= s.BlockNumber; {
			to := s.BlockNumber
			if s.BlockNumber-from >= r.limits.LogBlockRange {
				to = from + r.limits.LogBlockRange - 1
			}
			logs := e.logs(ethereum.FilterQuery{FromBlock: new(big.Int).SetUint64(from), ToBlock: new(big.Int).SetUint64(to), Addresses: []common.Address{result.Pool}, Topics: [][]common.Hash{{mint}}})
			e.discover(logs, ids)
			positions = e.positions(ids)
			if e.err != nil || covered(s.State, positions) || to == s.BlockNumber {
				break
			}
			from = to + 1
		}
	}
	result.Positions = positions
	if e.err != nil {
		return result, nil
	}
	if !covered(s.State, positions) {
		result.Reason = "incomplete_position_coverage"
		return result, nil
	}
	if len(positions) == 0 {
		result.Reason = "no_principal"
		return result, nil
	}
	if !e.coreCoverage(positions) {
		result.Reason = "incomplete_position_coverage"
		return result, nil
	}
	for i := range result.Positions {
		e.custody(&result.Positions[i])
		if e.err != nil {
			return result, nil
		}
	}
	e.header()
	if e.err != nil {
		return result, nil
	}
	observation, reason, calcErr := aggregate(s.State.SqrtPriceX96, result.Positions)
	if calcErr != nil {
		e.err = calcErr
		return result, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		e.err = fmt.Errorf("failed to finish lp protection: %w", ctxErr)
		return result, nil
	}
	result.Observation, result.Reason = observation, reason
	return result, nil
}

func (e *evaluation) discoverReceipts(ids map[string]*big.Int) {
	// Include the receipt acquired by identity() as well as caller-provided
	// receipts. A pool creation transaction commonly contains its first mint.
	available := make(map[common.Hash]*types.Receipt, len(e.req.Receipts)+len(e.receipts))
	for hash, receipt := range e.req.Receipts {
		available[hash] = receipt
	}
	for hash, receipt := range e.receipts {
		available[hash] = receipt
	}
	ordered := make([]common.Hash, 0, len(available))
	for hash := range available {
		ordered = append(ordered, hash)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Hex() < ordered[j].Hex() })
	mint := crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)"))
	count := 0
	for _, hash := range ordered {
		receipt := available[hash]
		if receipt == nil {
			continue
		}
		for _, l := range receipt.Logs {
			if e.err != nil {
				return
			}
			if l == nil || l.Address != e.result.Pool || len(l.Topics) == 0 || l.Topics[0] != mint {
				continue
			}
			if receipt.TxHash != hash || l.TxHash != hash {
				e.err = fmt.Errorf("failed to reuse lp receipt: identity=invalid")
				return
			}
			count++
			if count > e.r.limits.MaxLogs {
				e.err = fmt.Errorf("failed to reuse lp receipt: %w: logs=too_long", ErrBudget)
				return
			}
			e.discover([]types.Log{*l}, ids)
		}
	}
}

func (e *evaluation) identity() bool {
	req := e.req
	event := req.Creation
	if req.Protocol == clliquidity.V3 {
		e.factory = common.HexToAddress("0x33128a8fC17869897dcE68Ed026d694621f6FDfD")
		e.manager = common.HexToAddress("0x03a520b32C04BF3bEEf7BEb72E919cf822Ed34f1")
	} else {
		e.factory = common.HexToAddress("0xf8f2eB4940CFE7d13603DDDD87f123820Fc061Ef")
		e.manager = common.HexToAddress("0xe1f8cd9AC4e4A65F54f38a5CdAfCA44f6dD68b53")
	}
	e.result.Manager = e.manager
	if event.Address != e.factory {
		e.result.Reason = "unsupported_deployment"
		return false
	}
	sig := "PoolCreated(address,address,uint24,int24,address)"
	words := 2
	if req.Protocol == clliquidity.Slipstream {
		sig = "PoolCreated(address,address,int24,address)"
		words = 1
	}
	if len(event.Topics) != 4 || event.Topics[0] != crypto.Keccak256Hash([]byte(sig)) || len(event.Data) != words*32 || new(big.Int).SetBytes(event.Topics[1][:]).BitLen() > 160 || new(big.Int).SetBytes(event.Topics[2][:]).BitLen() > 160 || new(big.Int).SetBytes(event.Data[(words-1)*32:]).BitLen() > 160 || common.BytesToAddress(event.Data[(words-1)*32:]) != e.result.Pool {
		e.err = fmt.Errorf("failed to identify lp pool: creation_event=invalid")
		return false
	}
	e.token0, e.token1 = common.BytesToAddress(event.Topics[1][:]), common.BytesToAddress(event.Topics[2][:])
	e.third = new(big.Int).SetBytes(event.Topics[3][:])
	spacing := new(big.Int).Set(e.third)
	if req.Protocol == clliquidity.V3 {
		spacing = signed(new(big.Int).SetBytes(event.Data[:32]))
	}
	if spacing.Cmp(big.NewInt(int64(req.Principal.Snapshot.State.Spacing))) != 0 || e.token0 == (common.Address{}) || e.token0.Big().Cmp(e.token1.Big()) >= 0 {
		e.err = fmt.Errorf("failed to identify lp pool: coordinates=invalid")
		return false
	}
	if !e.attempt("eth_chainId") {
		return false
	}
	chain, err := e.r.rpc.ChainID(e.ctx)
	if err != nil {
		e.err = fmt.Errorf("failed to identify lp chain: %w", err)
		return false
	}
	if chain == nil || chain.Cmp(new(big.Int).SetUint64(req.ChainID)) != 0 {
		e.err = fmt.Errorf("failed to identify lp chain: chain=invalid")
		return false
	}
	if e.receipt(event) == nil {
		return false
	}
	getter := "getPool(address,address,uint24)"
	if req.Protocol == clliquidity.Slipstream {
		getter = "getPool(address,address,int24)"
	}
	if e.address(e.factory, getter, e.token0.Big(), e.token1.Big(), e.third) != e.result.Pool || e.address(e.manager, "factory()") != e.factory || e.address(e.result.Pool, "factory()") != e.factory {
		if e.err == nil {
			e.err = fmt.Errorf("failed to identify lp pool: factory_binding=invalid")
		}
		return false
	}
	// The caller supplies complete ticks. Bind the mutable portion to this pool
	// at the same hash to reject mixed observations before using that distribution.
	n := 7
	if req.Protocol == clliquidity.Slipstream {
		n = 6
	}
	slot := e.words(e.result.Pool, "slot0()", n)
	s := req.Principal.Snapshot.State
	if slot[0].Cmp(s.SqrtPriceX96) != 0 || signed(slot[1]).Cmp(big.NewInt(int64(s.Tick))) != 0 || e.word(e.result.Pool, "liquidity()").Cmp(s.ActiveLiquidity) != 0 {
		if e.err == nil {
			e.err = fmt.Errorf("failed to identify lp pool: principal_state=invalid")
		}
		return false
	}
	return e.err == nil
}

func (e *evaluation) discover(logs []types.Log, ids map[string]*big.Int) {
	mint := crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)"))
	increase := crypto.Keccak256Hash([]byte("IncreaseLiquidity(uint256,uint128,uint256,uint256)"))
	for _, l := range logs {
		if e.err != nil {
			return
		}
		if l.Removed || l.Address != e.result.Pool || l.BlockNumber < e.req.Creation.BlockNumber || l.BlockNumber > e.result.BlockNumber || len(l.Topics) != 4 || l.Topics[0] != mint || len(l.Data) != 128 {
			e.err = fmt.Errorf("failed to discover lp positions: mint_event=invalid")
			return
		}
		r := e.receipt(l)
		if r == nil {
			return
		}
		for _, event := range r.Logs {
			if event == nil || event.Removed || event.Address != e.manager || len(event.Topics) != 2 || event.Topics[0] != increase || len(event.Data) != 96 {
				continue
			}
			id := new(big.Int).SetBytes(event.Topics[1][:])
			if id.Sign() == 0 {
				continue
			}
			ids[id.String()] = id
			if len(ids) > e.r.limits.MaxPositions {
				e.err = fmt.Errorf("failed to discover lp positions: %w", ErrBudget)
				return
			}
		}
	}
}

func (e *evaluation) positions(ids map[string]*big.Int) []Position {
	ordered := make([]*big.Int, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Cmp(ordered[j]) < 0 })
	var result []Position
	for _, id := range ordered {
		v := e.words(e.manager, "positions(uint256)", 12, id)
		if e.err != nil {
			return result
		}
		if v[2].BitLen() > 160 || v[3].BitLen() > 160 {
			e.err = fmt.Errorf("failed to inspect lp position: token=invalid")
			return result
		}
		if common.BigToAddress(v[2]) != e.token0 || common.BigToAddress(v[3]) != e.token1 || v[4].Cmp(e.third) != 0 || v[7].Sign() == 0 {
			continue
		}
		lo, hi := signed(v[5]), signed(v[6])
		if !lo.IsInt64() || !hi.IsInt64() || lo.Int64() < -887272 || hi.Int64() > 887272 || lo.Cmp(hi) >= 0 || lo.Int64()%int64(e.req.Principal.Snapshot.State.Spacing) != 0 || hi.Int64()%int64(e.req.Principal.Snapshot.State.Spacing) != 0 || v[7].BitLen() > 128 {
			e.err = fmt.Errorf("failed to inspect lp position: coordinates=invalid")
			return result
		}
		result = append(result, Position{ID: new(big.Int).Set(id), Lower: int32(lo.Int64()), Upper: int32(hi.Int64()), Liquidity: v[7]})
	}
	return result
}

func (e *evaluation) coreCoverage(positions []Position) bool {
	ranges := make(map[[2]int32]*big.Int)
	for _, p := range positions {
		key := [2]int32{p.Lower, p.Upper}
		if ranges[key] == nil {
			ranges[key] = new(big.Int)
		}
		ranges[key].Add(ranges[key], p.Liquidity)
	}
	for ticks, liquidity := range ranges {
		packed := append([]byte{}, e.manager[:]...)
		for _, tick := range ticks {
			v := uint32(tick)
			packed = append(packed, byte(v>>16), byte(v>>8), byte(v))
		}
		key := crypto.Keccak256Hash(packed)
		// Both supported pools expose the five-word core Position.Info tuple.
		v := e.words(e.result.Pool, "positions(bytes32)", 5, new(big.Int).SetBytes(key[:]))
		if e.err != nil || v[0].Cmp(liquidity) != 0 {
			return false
		}
	}
	return true
}
