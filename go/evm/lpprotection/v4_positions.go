package lpprotection

import (
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func (e *evaluation) availableReceipts() map[common.Hash]*types.Receipt {
	// Persisted receipts retain the original tick range even after NFT burn
	// clears PositionInfo. They are discovery hints, rechecked against canonical
	// blocks and current salt-specific core state, never a protection verdict.
	available := make(map[common.Hash]*types.Receipt)
	for key := range e.evidence.data.Acquired {
		if !strings.HasPrefix(key, "receipt/") {
			continue
		}
		var receipt *types.Receipt
		e.cachedEvidence(key, &receipt)
		if e.err != nil {
			return nil
		}
		if receipt != nil {
			available[receipt.TxHash] = receipt
		}
	}
	for hash, receipt := range e.req.Receipts {
		available[hash] = receipt
	}
	for hash, receipt := range e.receipts {
		available[hash] = receipt
	}
	return available
}

func (e *evaluation) v4DiscoverReceipts(ids map[string]*big.Int) {
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
				return
			}
			if l == nil || l.Address != e.result.Pool || len(l.Topics) < 2 || l.Topics[0] != v4ModifyTopic() || l.Topics[1] != e.result.PoolID {
				continue
			}
			if r.TxHash != hash || l.TxHash != hash {
				e.err = fmt.Errorf("failed to reuse v4 receipt: identity=invalid")
				return
			}
			count++
			if count > e.r.limits.MaxLogs {
				e.err = fmt.Errorf("failed to reuse v4 receipt: %w: logs=too_long", ErrBudget)
				return
			}
			e.v4Discover([]types.Log{*l}, ids)
		}
	}
}

func (e *evaluation) v4Discover(logs []types.Log, ids map[string]*big.Int) {
	for _, l := range logs {
		if e.err != nil {
			return
		}
		if l.Removed || l.Address != e.result.Pool || l.BlockNumber < e.req.Creation.BlockNumber || l.BlockNumber > e.result.BlockNumber || l.BlockHash == (common.Hash{}) || l.TxHash == (common.Hash{}) || len(l.Topics) != 3 || l.Topics[0] != v4ModifyTopic() || l.Topics[1] != e.result.PoolID || l.Topics[2].Big().BitLen() > 160 || len(l.Data) != 128 {
			e.err = fmt.Errorf("failed to discover v4 positions: modify_event=invalid")
			return
		}
		if common.BytesToAddress(l.Topics[2][:]) != e.manager {
			// Another manager's salt is not an NFT ID. Its positive liquidity
			// will prevent complete coverage and therefore prevent publication.
			continue
		}
		lo := signed(new(big.Int).SetBytes(l.Data[:32]))
		hi := signed(new(big.Int).SetBytes(l.Data[32:64]))
		id := new(big.Int).SetBytes(l.Data[96:])
		if !e.v4ValidRange(lo, hi) || id.Sign() == 0 {
			e.err = fmt.Errorf("failed to discover v4 positions: coordinates=invalid")
			return
		}
		if e.receipt(l) == nil || !e.canonical(BlockReference{Number: l.BlockNumber, Hash: l.BlockHash}) {
			return
		}
		if e.v4Ranges == nil {
			e.v4Ranges = make(map[string][2]int32)
		}
		ticks := [2]int32{int32(lo.Int64()), int32(hi.Int64())}
		if previous, ok := e.v4Ranges[id.String()]; ok && previous != ticks {
			e.err = fmt.Errorf("failed to discover v4 positions: token_range=invalid")
			return
		}
		e.v4Ranges[id.String()] = ticks
		ids[id.String()] = id
		if len(ids) > e.r.limits.MaxPositions {
			e.err = fmt.Errorf("failed to discover v4 positions: %w", ErrBudget)
			return
		}
	}
}

func (e *evaluation) v4ValidRange(lo, hi *big.Int) bool {
	spacing := int64(e.req.Principal.Snapshot.State.Spacing)
	return lo.IsInt64() && hi.IsInt64() && lo.Int64() >= -887272 && hi.Int64() <= 887272 && lo.Cmp(hi) < 0 && spacing > 0 && lo.Int64()%spacing == 0 && hi.Int64()%spacing == 0
}

func (e *evaluation) v4CoreLiquidity(id *big.Int, ticks [2]int32) *big.Int {
	v := e.words(e.v4StateView, "getPositionInfo(bytes32,address,int24,int24,bytes32)", 3, e.result.PoolID.Big(), e.manager.Big(), big.NewInt(int64(ticks[0])), big.NewInt(int64(ticks[1])), id)
	if e.err == nil && v[0].BitLen() > 128 {
		e.err = fmt.Errorf("failed to inspect v4 core position: liquidity=invalid")
	}
	return v[0]
}

func (e *evaluation) v4Positions(ids map[string]*big.Int) []Position {
	ordered := make([]*big.Int, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Cmp(ordered[j]) < 0 })
	var result []Position
	for _, id := range ordered {
		if e.err != nil {
			return result
		}
		hint, hasHint := e.v4Ranges[id.String()]
		if hasHint {
			liquidity := e.v4CoreLiquidity(id, hint)
			if e.err != nil {
				return result
			}
			if liquidity.Sign() == 0 {
				// Only canonical range + salt and an explicit core zero permits
				// this shortcut. A getter revert is never treated as a zero.
				continue
			}
		}
		v := e.words(e.manager, "getPoolAndPositionInfo(uint256)", 6, id)
		if e.err != nil {
			return result
		}
		if v[5].Sign() == 0 {
			e.err = fmt.Errorf("failed to inspect v4 position: position_info=empty")
			return result
		}
		if v[0].BitLen() > 160 || v[1].BitLen() > 160 || v[2].BitLen() > 24 || v[4].BitLen() > 160 {
			e.err = fmt.Errorf("failed to inspect v4 position: pool_key=invalid")
			return result
		}
		poolID := crypto.Keccak256Hash(query("poolKey()", v[:5]...)[4:])
		if poolID != e.result.PoolID {
			if hasHint {
				e.err = fmt.Errorf("failed to inspect v4 position: pool_binding=invalid")
				return result
			}
			continue
		}
		prefix := new(big.Int).Rsh(new(big.Int).Set(v[5]), 56)
		if prefix.Cmp(new(big.Int).Rsh(poolID.Big(), 56)) != 0 || v[5].Uint64()&0xff > 1 {
			e.err = fmt.Errorf("failed to inspect v4 position: packed_info=invalid")
			return result
		}
		lo, hi := v4PackedTick(v[5], 8), v4PackedTick(v[5], 32)
		if !e.v4ValidRange(big.NewInt(int64(lo)), big.NewInt(int64(hi))) || hasHint && hint != [2]int32{lo, hi} {
			e.err = fmt.Errorf("failed to inspect v4 position: coordinates=invalid")
			return result
		}
		liquidity := e.word(e.manager, "getPositionLiquidity(uint256)", id)
		core := e.v4CoreLiquidity(id, [2]int32{lo, hi})
		if e.err != nil {
			return result
		}
		if liquidity.BitLen() > 128 || liquidity.Cmp(core) != 0 {
			e.err = fmt.Errorf("failed to inspect v4 position: core_liquidity=invalid")
			return result
		}
		if liquidity.Sign() != 0 {
			result = append(result, Position{ID: new(big.Int).Set(id), Lower: lo, Upper: hi, Liquidity: liquidity})
		}
	}
	return result
}

func v4PackedTick(info *big.Int, offset uint) int32 {
	v := new(big.Int).Rsh(new(big.Int).Set(info), offset).Uint64() & 0xffffff
	if v&0x800000 != 0 {
		return int32(v) - 1<<24
	}
	return int32(v)
}
