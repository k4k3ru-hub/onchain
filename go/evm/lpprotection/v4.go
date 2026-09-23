package lpprotection

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v4/deployment"
)

// Full deployed runtime, including immutable bindings and metadata. See
// V4.md for the independently compiled source and fixture provenance.
const v4PositionManagerSHA256 = "d7b02189fef5e67af05a49e12148357e95baa29cf04066985807864d25598010"

func v4ModifyTopic() common.Hash {
	return crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)"))
}

func (e *evaluation) v4Identity() bool {
	d, err := deployment.ByChainID(e.req.ChainID)
	if err != nil {
		e.err = fmt.Errorf("failed to identify v4 deployment: %w", err)
		return false
	}
	l := e.req.Creation
	if e.result.Pool != d.PoolManager || l.Address != d.PoolManager || d.PositionManager == (common.Address{}) {
		e.result.Reason = "unsupported_deployment"
		return false
	}
	e.manager, e.v4StateView = d.PositionManager, d.StateView
	e.result.Manager = e.manager
	initialize := crypto.Keccak256Hash([]byte("Initialize(bytes32,address,address,uint24,int24,address,uint160,int24)"))
	if len(l.Topics) != 4 || l.Topics[0] != initialize || len(l.Data) != 160 || e.result.PoolID == (common.Hash{}) || l.Topics[1] != e.result.PoolID || l.Topics[2].Big().BitLen() > 160 || l.Topics[3].Big().BitLen() > 160 {
		e.err = fmt.Errorf("failed to identify v4 pool: creation_event=invalid")
		return false
	}
	e.token0, e.token1 = common.BytesToAddress(l.Topics[2][:]), common.BytesToAddress(l.Topics[3][:])
	fee := new(big.Int).SetBytes(l.Data[:32])
	spacing := signed(new(big.Int).SetBytes(l.Data[32:64]))
	hooks := new(big.Int).SetBytes(l.Data[64:96])
	price := new(big.Int).SetBytes(l.Data[96:128])
	tick := signed(new(big.Int).SetBytes(l.Data[128:]))
	if e.token0.Big().Cmp(e.token1.Big()) >= 0 || fee.BitLen() > 24 || hooks.BitLen() > 160 || !spacing.IsInt64() || spacing.Sign() <= 0 || spacing.Int64() > 32767 || spacing.Int64() != int64(e.req.Principal.Snapshot.State.Spacing) || price.Sign() <= 0 || price.BitLen() > 160 || !tick.IsInt64() || tick.Int64() < -887272 || tick.Int64() > 887272 {
		e.err = fmt.Errorf("failed to identify v4 pool: coordinates=invalid")
		return false
	}
	key := query("poolKey()", e.token0.Big(), e.token1.Big(), fee, spacing, hooks)[4:]
	if crypto.Keccak256Hash(key) != e.result.PoolID {
		e.err = fmt.Errorf("failed to identify v4 pool: pool_id=invalid")
		return false
	}
	if hooks.Sign() != 0 || fee.Cmp(big.NewInt(1000000)) > 0 {
		e.result.Reason = "unsupported_pool_configuration"
		return false
	}
	e.third = fee
	if !e.attempt("eth_chainId") {
		return false
	}
	chain, err := e.r.rpc.ChainID(e.ctx)
	if err != nil {
		e.err = fmt.Errorf("failed to identify v4 chain: %w", err)
		return false
	}
	if chain == nil || chain.Cmp(new(big.Int).SetUint64(e.req.ChainID)) != 0 {
		e.err = fmt.Errorf("failed to identify v4 chain: chain=invalid")
		return false
	}
	if e.receipt(l) == nil || !e.canonical(BlockReference{Number: l.BlockNumber, Hash: l.BlockHash}) {
		return false
	}
	code := e.code(e.manager)
	if e.err != nil {
		return false
	}
	sum := sha256.Sum256(code)
	if len(code) != 23877 || hex.EncodeToString(sum[:]) != v4PositionManagerSHA256 {
		e.result.Reason = "unsupported_position_manager_code"
		return false
	}
	if e.address(e.manager, "poolManager()") != d.PoolManager || e.address(e.v4StateView, "poolManager()") != d.PoolManager {
		if e.err == nil {
			e.err = fmt.Errorf("failed to identify v4 pool: manager_binding=invalid")
		}
		return false
	}
	slot := e.words(e.v4StateView, "getSlot0(bytes32)", 4, e.result.PoolID.Big())
	s := e.req.Principal.Snapshot.State
	if slot[0].Cmp(s.SqrtPriceX96) != 0 || signed(slot[1]).Cmp(big.NewInt(int64(s.Tick))) != 0 || slot[2].BitLen() > 24 || slot[3].Cmp(fee) != 0 || e.word(e.v4StateView, "getLiquidity(bytes32)", e.result.PoolID.Big()).Cmp(s.ActiveLiquidity) != 0 {
		if e.err == nil {
			e.err = fmt.Errorf("failed to identify v4 pool: principal_state=invalid")
		}
		return false
	}
	return e.err == nil
}

func (e *evaluation) v4Custody(p *Position, code []byte) {
	if !ordinary(p.Owner, code) {
		p.Reason = "unsupported_custody_code"
		return
	}
	// Execute both full principal decrease and settlement. A zero decrease
	// would test fee collection only and must never stand in for withdrawal.
	data := v4WithdrawalData(p, e.token0, e.token1, e.result.BlockTime.Unix()+3600)
	response := e.call(p.Owner, e.manager, data)
	if e.err == nil && len(response) != 0 {
		e.err = fmt.Errorf("failed to verify v4 withdrawal: response=invalid")
	}
	if e.err == nil {
		p.Kind = "withdrawable"
		p.Model = "v4-direct-eoa-v1"
		p.CanWeakenProtection = flag(false)
	}
}

func v4WithdrawalData(p *Position, token0, token1 common.Address, deadline int64) []byte {
	// Fixed ABI: (tokenId, full liquidity, min0, min1, empty hookData).
	decrease := query("params()", p.ID, p.Liquidity, new(big.Int), new(big.Int), big.NewInt(160), new(big.Int))[4:]
	take := query("params()", token0.Big(), token1.Big(), p.Owner.Big())[4:]
	// bytes[] has two offsets relative to the first element-offset word.
	params := query("params()", big.NewInt(2), big.NewInt(64), big.NewInt(288), big.NewInt(int64(len(decrease))))[4:]
	params = append(params, decrease...)
	params = append(params, query("params()", big.NewInt(int64(len(take))))[4:]...)
	params = append(params, take...)
	// abi.encode(bytes actions, bytes[] params), DECREASE_LIQUIDITY + TAKE_PAIR.
	unlock := query("params()", big.NewInt(64), big.NewInt(128), big.NewInt(2))[4:]
	actions := make([]byte, 32)
	actions[0], actions[1] = 0x01, 0x11
	unlock = append(unlock, actions...)
	unlock = append(unlock, params...)
	data := query("modifyLiquidities(bytes,uint256)", big.NewInt(64), big.NewInt(deadline), big.NewInt(int64(len(unlock))))
	return append(data, unlock...)
}
