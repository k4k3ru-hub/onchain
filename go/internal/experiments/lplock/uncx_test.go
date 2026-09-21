// Package lplock contains opt-in, read-only feasibility checks, not a public API.
package lplock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/k4k3ru-hub/onchain/go/evm"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v3/protocol"
)

const (
	baseRPC         = "https://mainnet.base.org"
	reviewedRuntime = "0x865de3c33580ca7144492890feee24ef985af705497e92e1dea0ef11a8be0eb8"
	reviewedSource  = "12116e83ad4a3a53946d65ea783fac4d337b5b23e955525ffd6c71733ebf2f80"
	lockSignature   = "onLock(uint256,address,uint256,address,address,address,uint256,uint16,uint256,address,(uint96,address,address,address,uint24,int24,int24,uint128,uint256,uint256,uint128,uint128))"
)

type measuredRPC struct {
	sdk      *evm.HTTPClient
	raw      *ethclient.Client
	last     time.Time
	methods  map[string]int
	retries  int
	pinBlock *big.Int
	retryAll bool
}

func (p *measuredRPC) invoke(ctx context.Context, method string, fn func() error) error {
	for attempt := 0; attempt < 4; attempt++ {
		interval := 1500 * time.Millisecond
		if p.retryAll && attempt > 0 {
			interval = time.Duration(1<<attempt) * time.Second
		}
		if delay := time.Until(p.last.Add(interval)); delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("failed to schedule probe read: %w", ctx.Err())
			case <-timer.C:
			}
		}
		p.last = time.Now()
		p.methods[method]++
		err := fn()
		var httpErr rpc.HTTPError
		limited := errors.As(err, &httpErr) && httpErr.StatusCode == 429
		_, revertErr := probeRevert(err)
		retry := limited || (p.retryAll && err != nil && revertErr != nil && ctx.Err() == nil)
		if !retry || attempt == 3 {
			return err
		}
		if limited {
			p.retries++
		}
	}
	return fmt.Errorf("failed to read probe data: attempts=out_of_range")
}

// HeaderByNumber reads a pinned header or finalized when no reference pin is set.
//
// Version:
//   - 2026-09-20: Support a reference block for independent-result comparison.
func (p *measuredRPC) HeaderByNumber(ctx context.Context, n *big.Int) (h *types.Header, err error) {
	if n == nil {
		if p.pinBlock != nil {
			n = new(big.Int).Set(p.pinBlock)
		} else {
			n = big.NewInt(int64(rpc.FinalizedBlockNumber))
		}
	}
	err = p.invoke(ctx, "eth_getBlockByNumber", func() error { h, err = p.raw.HeaderByNumber(ctx, n); return err })
	return h, err
}

// CallContract delegates read-only calls to the existing SDK and records usage.
//
// Version:
//   - 2026-09-20: Added.
func (p *measuredRPC) CallContract(ctx context.Context, m ethereum.CallMsg, n *big.Int) (b []byte, err error) {
	err = p.invoke(ctx, "eth_call", func() error { b, err = p.sdk.CallContract(ctx, m, n); return err })
	return b, err
}

func word(n *big.Int) []byte {
	v := new(big.Int).Mod(new(big.Int).Set(n), new(big.Int).Lsh(big.NewInt(1), 256))
	return v.FillBytes(make([]byte, 32))
}

func query(signature string, args ...*big.Int) []byte {
	data := append([]byte(nil), crypto.Keccak256([]byte(signature))[:4]...)
	for _, arg := range args {
		data = append(data, word(arg)...)
	}
	return data
}

func decodeWords(raw []byte, count int) ([]*big.Int, error) {
	if len(raw) != count*32 {
		return nil, fmt.Errorf("failed to decode probe words: data=invalid actual_length=%d expected_length=%d", len(raw), count*32)
	}
	out := make([]*big.Int, count)
	for i := range out {
		out[i] = new(big.Int).SetBytes(raw[i*32 : (i+1)*32])
	}
	return out, nil
}

func signedTick(n *big.Int) int32 {
	v := int32(n.Uint64() & 0xffffff)
	if v&0x800000 != 0 {
		v -= 1 << 24
	}
	return v
}

func fullSinglePosition(s clliquidity.State, lower, upper int32, liquidity, core *big.Int) bool {
	if !s.Complete || liquidity == nil || core == nil || liquidity.Sign() <= 0 || core.Cmp(liquidity) != 0 || lower >= upper || len(s.Ticks) != 2 {
		return false
	}
	if _, err := clliquidity.Calculate(s); err != nil {
		return false
	}
	a, b := s.Ticks[0], s.Ticks[1]
	return a.Index == lower && b.Index == upper && a.Gross.Cmp(liquidity) == 0 && b.Gross.Cmp(liquidity) == 0 && a.Net.Cmp(liquidity) == 0 && b.Net.Cmp(new(big.Int).Neg(liquidity)) == 0
}

func discoverLock(logs []*types.Log, owner, manager, pool common.Address, tokenID *big.Int) (*big.Int, error) {
	var found *big.Int
	for _, log := range logs {
		if log == nil || log.Removed || log.Address != owner || len(log.Topics) != 1 || log.Topics[0] != crypto.Keccak256Hash([]byte(lockSignature)) {
			continue
		}
		w, err := decodeWords(log.Data, 22)
		if err != nil {
			return nil, fmt.Errorf("failed to discover lock event: %w", err)
		}
		if common.BigToAddress(w[1]) != manager || common.BigToAddress(w[9]) != pool || w[2].Cmp(tokenID) != 0 {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("failed to discover lock event: matching_events=ambiguous")
		}
		found = w[0]
	}
	if found == nil {
		return nil, fmt.Errorf("failed to discover lock event: matching_event=empty")
	}
	return found, nil
}

func decodeRevert(err error) (string, error) {
	var dataErr rpc.DataError
	if !errors.As(err, &dataErr) {
		return "", fmt.Errorf("failed to decode contract revert: %w", err)
	}
	encoded, ok := dataErr.ErrorData().(string)
	if !ok {
		return "", fmt.Errorf("failed to decode contract revert: data=invalid")
	}
	raw, decodeErr := hexutil.Decode(encoded)
	if decodeErr != nil {
		return "", fmt.Errorf("failed to decode contract revert: %w", decodeErr)
	}
	reason, decodeErr := abi.UnpackRevert(raw)
	if decodeErr != nil {
		return "", fmt.Errorf("failed to decode contract revert: %w", decodeErr)
	}
	return reason, nil
}

func verifySource(ctx context.Context, owner common.Address, code []byte) error {
	if crypto.Keccak256Hash(code) != common.HexToHash(reviewedRuntime) {
		return fmt.Errorf("failed to verify locker source: runtime=unreviewed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://sourcify.dev/server/v2/contract/8453/"+owner.Hex()+"?fields=all", nil)
	if err != nil {
		return fmt.Errorf("failed to create source request: %w", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to retrieve locker source: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 4_000_001))
	closeErr := response.Body.Close()
	if readErr != nil {
		return fmt.Errorf("failed to read locker source: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close locker source: %w", closeErr)
	}
	if response.StatusCode != 200 || len(raw) > 4_000_000 {
		return fmt.Errorf("failed to retrieve locker source: response=invalid status_code=%d", response.StatusCode)
	}
	var source struct {
		RuntimeMatch    string `json:"runtimeMatch"`
		RuntimeBytecode struct {
			OnchainBytecode string `json:"onchainBytecode"`
		} `json:"runtimeBytecode"`
		Sources map[string]struct {
			Content string `json:"content"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		return fmt.Errorf("failed to decode locker source: %w", err)
	}
	deployed, err := hexutil.Decode(source.RuntimeBytecode.OnchainBytecode)
	if err != nil {
		return fmt.Errorf("failed to decode verified runtime: %w", err)
	}
	main := source.Sources["contracts/UNCX_LiquidityLocker_UniV3.sol"].Content
	if source.RuntimeMatch != "exact_match" || !bytes.Equal(deployed, code) || fmt.Sprintf("%x", sha256.Sum256([]byte(main))) != reviewedSource {
		return fmt.Errorf("failed to verify locker source: source_runtime=mismatch")
	}
	return nil
}

// TestUNCXPoolFirstLive verifies reviewed withdrawal paths using real chain state.
// It is explicitly opt-in and never sends a transaction.
//
// Version:
//   - 2026-09-20: Allow a pinned block for use as a separate reference check.
func TestUNCXPoolFirstLive(t *testing.T) {
	if os.Getenv("ONCHAIN_UNCX_LIVE") != "1" {
		t.Skip("set ONCHAIN_UNCX_LIVE=1 for public RPC reads")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	started := time.Now()
	sdk, err := evm.NewHTTPClient(ctx, evm.HTTPConfig{URL: baseRPC})
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	raw, err := ethclient.DialContext(ctx, baseRPC)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	p := &measuredRPC{sdk: sdk, raw: raw, methods: make(map[string]int)}
	if referenceBlock := os.Getenv("ONCHAIN_REFERENCE_BLOCK"); referenceBlock != "" {
		pin, ok := new(big.Int).SetString(referenceBlock, 10)
		if !ok || !pin.IsUint64() {
			t.Fatal("failed to pin reference block: block=invalid")
		}
		p.pinBlock = pin
	}
	factory := common.HexToAddress("0x33128a8fc17869897dce68ed026d694621f6fdfd")
	manager := common.HexToAddress("0x03a520b32c04bf3beef7beb72e919cf822ed34f1")
	input, err := os.ReadFile("testdata/pool_created.json")
	if err != nil {
		t.Fatal(err)
	}
	var event types.Log
	if err := json.Unmarshal(input, &event); err != nil {
		t.Fatal(err)
	}
	if event.Removed || event.Address != factory || len(event.Topics) != 4 || event.Topics[0] != crypto.Keccak256Hash([]byte("PoolCreated(address,address,uint24,int24,address)")) {
		t.Fatal("failed to validate creation input: event=invalid")
	}
	creation, err := decodeWords(event.Data, 2)
	if err != nil {
		t.Fatal(err)
	}
	pool := common.BigToAddress(creation[1])
	spacing := int32(creation[0].Int64())
	key := protocol.PoolKey{Token0: protocol.NewCurrency(common.BytesToAddress(event.Topics[1][:])), Token1: protocol.NewCurrency(common.BytesToAddress(event.Topics[2][:])), Fee: uint32(event.Topics[3].Big().Uint64())}
	derived, err := key.Address(factory, common.HexToHash("0xe34f199b19b2b4f47f68442619d555527d244f78a3297ea89325f843f87b8b54"))
	if err != nil {
		t.Fatal(err)
	}
	if derived != pool {
		t.Fatal("failed to validate creation input: pool=mismatch")
	}
	var receipt *types.Receipt
	err = p.invoke(ctx, "eth_getTransactionReceipt", func() error { receipt, err = sdk.TransactionReceipt(ctx, event.TxHash); return err })
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != 1 || receipt.BlockHash != event.BlockHash || receipt.BlockNumber.Uint64() != event.BlockNumber {
		t.Fatal("failed to validate creation receipt: receipt=mismatch")
	}
	creationHeader, err := p.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		t.Fatal(err)
	}
	if creationHeader.Hash() != event.BlockHash {
		t.Fatal("failed to validate creation header: block=mismatch")
	}
	found, managerMint := false, false
	ids := make(map[string]*big.Int)
	for _, log := range receipt.Logs {
		if log.Address == factory && log.Index == event.Index && bytes.Equal(log.Data, event.Data) && len(log.Topics) == 4 {
			found = true
			for i := range log.Topics {
				if log.Topics[i] != event.Topics[i] {
					found = false
				}
			}
		}
		if log.Address == pool && len(log.Topics) == 4 && log.Topics[0] == crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)")) && common.BytesToAddress(log.Topics[1][:]) == manager {
			managerMint = true
		}
		if log.Address == manager && len(log.Topics) == 2 && log.Topics[0] == crypto.Keccak256Hash([]byte("IncreaseLiquidity(uint256,uint128,uint256,uint256)")) {
			ids[log.Topics[1].Hex()] = log.Topics[1].Big()
		}
	}
	if !found || !managerMint || len(ids) == 0 || len(ids) > 16 {
		t.Fatal("failed to discover pool positions: creation_mints=unsupported")
	}
	reader, err := clliquidity.NewReader(p, clliquidity.Limits{Timeout: time.Minute, ChunkSize: 64, MaxReads: 256, MaxCalls: 24, MaxTicks: 64, MaxResponseBytes: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reader.Capture(ctx, clliquidity.Pool{Protocol: clliquidity.V3, Address: pool, Spacing: spacing, Multicall: common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")})
	if err != nil {
		t.Fatal(err)
	}
	block := new(big.Int).SetUint64(snapshot.BlockNumber)
	call := func(target common.Address, signature string, count int, args ...*big.Int) []*big.Int {
		t.Helper()
		data, err := p.CallContract(ctx, ethereum.CallMsg{To: &target, Data: query(signature, args...)}, block)
		if err != nil {
			t.Fatal(err)
		}
		out, err := decodeWords(data, count)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	var tokenID *big.Int
	var position []*big.Int
	for _, id := range ids {
		w := call(manager, "positions(uint256)", 12, id)
		if common.BigToAddress(w[2]) != key.Token0.Address() || common.BigToAddress(w[3]) != key.Token1.Address() || w[4].Uint64() != uint64(key.Fee) || w[7].Sign() == 0 {
			continue
		}
		if tokenID != nil {
			t.Fatal("failed to verify sample coverage: multiple_positions=unsupported")
		}
		tokenID, position = id, w
	}
	if tokenID == nil {
		t.Fatal("failed to discover pool positions: nonzero_position=empty")
	}
	lower, upper := signedTick(position[5]), signedTick(position[6])
	packed := append(append(append([]byte(nil), manager[:]...), word(big.NewInt(int64(lower)))[29:]...), word(big.NewInt(int64(upper)))[29:]...)
	core := call(pool, "positions(bytes32)", 5, crypto.Keccak256Hash(packed).Big())
	if !fullSinglePosition(snapshot.State, lower, upper, position[7], core[0]) {
		t.Fatal("failed to verify sample coverage: liquidity=incomplete")
	}
	owner := common.BigToAddress(call(manager, "ownerOf(uint256)", 1, tokenID)[0])
	approval := common.BigToAddress(call(manager, "getApproved(uint256)", 1, tokenID)[0])
	if approval != (common.Address{}) {
		t.Fatal("failed to verify custody: approval=unresolved")
	}
	var code []byte
	err = p.invoke(ctx, "eth_getCode", func() error { code, err = raw.CodeAt(ctx, owner, block); return err })
	if err != nil {
		t.Fatal(err)
	}
	if err := verifySource(ctx, owner, code); err != nil {
		t.Fatal(err)
	}
	lockID, err := discoverLock(receipt.Logs, owner, manager, pool, tokenID)
	if err != nil {
		t.Fatal(err)
	}
	lock := call(owner, "getLock(uint256)", 11, lockID)
	if lock[0].Cmp(lockID) != 0 || common.BigToAddress(lock[1]) != manager || common.BigToAddress(lock[2]) != pool || lock[3].Cmp(tokenID) != 0 {
		t.Fatal("failed to verify lock identity: lock=mismatch")
	}
	lockOwner := common.BigToAddress(lock[4])
	if lockOwner == (common.Address{}) || lock[8].Cmp(big.NewInt(snapshot.BlockTime.Unix())) < 0 {
		t.Fatal("failed to verify sample lock: current_lock=unavailable")
	}
	migrator := common.BigToAddress(call(owner, "MIGRATOR()", 1)[0])
	admin := common.BigToAddress(call(owner, "owner()", 1)[0])
	if migrator != (common.Address{}) {
		t.Fatal("failed to verify sample lock: migration=unresolved")
	}
	checks := make(map[string]string)
	revert := func(name, signature, expected string, target, sender common.Address, args ...*big.Int) {
		t.Helper()
		_, err := p.CallContract(ctx, ethereum.CallMsg{To: &target, From: sender, Data: query(signature, args...)}, block)
		if err == nil {
			t.Fatalf("failed to verify withdrawal restriction: unexpected_success check=%q", name)
		}
		reason, err := decodeRevert(err)
		if err != nil {
			t.Fatal(err)
		}
		if reason != expected {
			t.Fatalf("failed to verify withdrawal restriction: reason=mismatch check=%q reason=%q", name, reason)
		}
		checks[name] = reason
	}
	deadline := big.NewInt(snapshot.BlockTime.Unix() + 60)
	zero := new(big.Int)
	revert("withdraw", "withdraw(uint256,address)", "NOT YET", owner, lockOwner, lockID, lock[4])
	revert("decrease_liquidity", "decreaseLiquidity(uint256,(uint256,uint128,uint256,uint256,uint256))", "NOT YET", owner, lockOwner, lockID, tokenID, position[7], zero, zero, deadline)
	revert("migrate", "migrate(uint256)", "NOT SET", owner, lockOwner, lockID)
	revert("direct_nft_decrease", "decreaseLiquidity((uint256,uint128,uint256,uint256,uint256))", "Not approved", manager, lockOwner, tokenID, position[7], zero, zero, deadline)
	revert("shorten_lock", "relock(uint256,uint256)", "DATE", owner, lockOwner, lockID, new(big.Int).Sub(lock[8], big.NewInt(1)))
	if admin != (common.Address{}) {
		revert("admin_refund_nft", "adminRefundERC20(address,address,uint256)", "ST", owner, admin, new(big.Int).SetBytes(manager[:]), lock[4], tokenID)
		// Simulate the admin's actual configuration authority, without persisting it.
		if _, err := p.CallContract(ctx, ethereum.CallMsg{To: &owner, From: admin, Data: query("setMigrator(address)", lock[4])}, block); err != nil {
			t.Fatal(err)
		}
		checks["admin_set_migrator"] = "success_without_persisted_state"
	}
	after, err := p.HeaderByNumber(ctx, block)
	if err != nil {
		t.Fatal(err)
	}
	if after.Hash() != snapshot.BlockHash {
		t.Fatal("failed to verify evaluation block: block=mismatch")
	}
	result := map[string]any{
		"pool": pool.Hex(), "creation_transaction": event.TxHash.Hex(), "block_number": snapshot.BlockNumber, "block_hash": snapshot.BlockHash.Hex(),
		"token_id": tokenID.String(), "custodian": owner.Hex(), "lock_id": lockID.String(), "unlock_date": lock[8].String(),
		"lock_owner": lockOwner.Hex(), "migrator": migrator.Hex(), "admin_address": admin.Hex(),
		"percentage": "100", "basis": "complete_lp_principal_current_configuration", "admin_can_set_migrator": admin != (common.Address{}),
		"principal_token0": snapshot.Amounts.Token0.String(), "principal_token1": snapshot.Amounts.Token1.String(), "runtime_hash": reviewedRuntime, "source_sha256": reviewedSource,
		"checks": checks, "rpc_http_attempts": p.methods, "rate_limit_retries": p.retries, "source_http_requests": 1, "wall_seconds": time.Since(started).Seconds(),
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
	if path := os.Getenv("ONCHAIN_UNCX_REPORT"); path != "" {
		if err := os.WriteFile(path, append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
