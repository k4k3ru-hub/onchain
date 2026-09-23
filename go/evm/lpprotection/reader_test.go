package lpprotection

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

type fakeRPC struct {
	t                       *testing.T
	header                  *types.Header
	chain                   *big.Int
	receipts                map[common.Hash]*types.Receipt
	codes                   map[common.Address][]byte
	responses               map[string][]byte
	logs                    []types.Log
	approvalLogs            []types.Log
	historyErr, approvalErr error
	queries                 []ethereum.FilterQuery
	called                  []string
	fail                    error
	hash                    common.Hash
}

// ChainID supplies the fixture chain.
//
// Version:
//   - 2026-09-23: Added.
func (f *fakeRPC) ChainID(context.Context) (*big.Int, error) {
	f.called = append(f.called, "chain")
	return f.chain, f.fail
}

// HeaderByNumber supplies deterministic fixture headers for checkpoint tests.
//
// Version:
//   - 2026-09-23: Added.
func (f *fakeRPC) HeaderByNumber(_ context.Context, n *big.Int) (*types.Header, error) {
	if n.Cmp(f.header.Number) != 0 {
		h := types.CopyHeader(f.header)
		h.Number = new(big.Int).Set(n)
		f.called = append(f.called, "header")
		return h, f.fail
	}
	f.called = append(f.called, "header")
	return f.header, f.fail
}

// CallContractAtHash returns a response only for the exact fixture call.
//
// Version:
//   - 2026-09-23: Added.
func (f *fakeRPC) CallContractAtHash(_ context.Context, msg ethereum.CallMsg, h common.Hash) ([]byte, error) {
	if h != f.hash {
		f.t.Fatal("unpinned contract call")
	}
	k := callKey(msg.From, *msg.To, msg.Data)
	f.called = append(f.called, k)
	if f.fail != nil {
		return nil, f.fail
	}
	v, ok := f.responses[k]
	if !ok {
		f.t.Fatalf("unexpected call to %s data=%x from=%s", msg.To.Hex(), msg.Data, msg.From.Hex())
	}
	return v, nil
}

// CodeAtHash returns fixture runtime at the required hash.
//
// Version:
//   - 2026-09-23: Added.
func (f *fakeRPC) CodeAtHash(_ context.Context, a common.Address, h common.Hash) ([]byte, error) {
	if h != f.hash {
		f.t.Fatal("unpinned code")
	}
	f.called = append(f.called, "code")
	return f.codes[a], f.fail
}

// FilterLogs returns bounded mint or operator events from the fixture.
//
// Version:
//   - 2026-09-23: Separate operator history from pool mint history.
func (f *fakeRPC) FilterLogs(_ context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	f.called = append(f.called, "logs")
	f.queries = append(f.queries, q)
	if f.fail != nil {
		return nil, f.fail
	}
	logs, err := f.logs, f.historyErr
	if len(q.Topics) > 0 && len(q.Topics[0]) > 0 && q.Topics[0][0] == crypto.Keccak256Hash([]byte("ApprovalForAll(address,address,bool)")) {
		logs, err = f.approvalLogs, f.approvalErr
	}
	if err != nil {
		return nil, err
	}
	var result []types.Log
	for _, l := range logs {
		if new(big.Int).SetUint64(l.BlockNumber).Cmp(q.FromBlock) >= 0 && new(big.Int).SetUint64(l.BlockNumber).Cmp(q.ToBlock) <= 0 {
			result = append(result, l)
		}
	}
	return result, nil
}

// TransactionReceipt returns the requested fixture receipt.
//
// Version:
//   - 2026-09-23: Added.
func (f *fakeRPC) TransactionReceipt(_ context.Context, h common.Hash) (*types.Receipt, error) {
	f.called = append(f.called, "receipt")
	return f.receipts[h], f.fail
}

func callKey(from, to common.Address, data []byte) string {
	return from.Hex() + to.Hex() + hex.EncodeToString(data)
}
func words(values ...*big.Int) []byte { return query("fixture()", values...)[4:] }
func (f *fakeRPC) put(to common.Address, sig string, result []byte, args ...*big.Int) {
	f.responses[callKey(common.Address{}, to, query(sig, args...))] = result
}
func (f *fakeRPC) integer(to common.Address, sig string, v *big.Int, args ...*big.Int) {
	f.put(to, sig, words(v), args...)
}

func fixture(t *testing.T, protocol clliquidity.Protocol) (*fakeRPC, Request, common.Address) {
	t.Helper()
	f := &fakeRPC{t: t, header: &types.Header{Number: big.NewInt(100), Time: 1800000000}, chain: big.NewInt(8453), receipts: make(map[common.Hash]*types.Receipt), codes: make(map[common.Address][]byte), responses: make(map[string][]byte)}
	f.hash = f.header.Hash()
	factory := common.HexToAddress("0x33128a8fC17869897dcE68Ed026d694621f6FDfD")
	manager := common.HexToAddress("0x03a520b32C04BF3bEEf7BEb72E919cf822Ed34f1")
	pool := common.HexToAddress("0x100001")
	token0, token1 := common.HexToAddress("0x200001"), common.HexToAddress("0x200002")
	signature := "PoolCreated(address,address,uint24,int24,address)"
	third := big.NewInt(3000)
	data := words(big.NewInt(60), pool.Big())
	get := "getPool(address,address,uint24)"
	slotWords := 7
	if protocol == clliquidity.Slipstream {
		factory = common.HexToAddress("0xf8f2eB4940CFE7d13603DDDD87f123820Fc061Ef")
		manager = common.HexToAddress("0xe1f8cd9AC4e4A65F54f38a5CdAfCA44f6dD68b53")
		signature = "PoolCreated(address,address,int24,address)"
		third = big.NewInt(60)
		data = words(pool.Big())
		get = "getPool(address,address,int24)"
		slotWords = 6
	}
	creation := types.Log{Address: factory, Topics: []common.Hash{crypto.Keccak256Hash([]byte(signature)), common.BigToHash(token0.Big()), common.BigToHash(token1.Big()), common.BigToHash(third)}, Data: data, BlockNumber: 99, BlockHash: common.HexToHash("0x11"), TxHash: common.HexToHash("0x22")}
	mint := types.Log{Address: pool, Topics: []common.Hash{crypto.Keccak256Hash([]byte("Mint(address,address,int24,int24,uint128,uint256,uint256)")), common.BigToHash(manager.Big()), {}, {}}, Data: make([]byte, 128), BlockNumber: 99, BlockHash: creation.BlockHash, TxHash: creation.TxHash, Index: 1}
	increase := types.Log{Address: manager, Topics: []common.Hash{crypto.Keccak256Hash([]byte("IncreaseLiquidity(uint256,uint128,uint256,uint256)")), common.BigToHash(big.NewInt(1))}, Data: make([]byte, 96), BlockNumber: 99, BlockHash: creation.BlockHash, TxHash: creation.TxHash, Index: 2}
	f.receipts[creation.TxHash] = &types.Receipt{Status: 1, BlockNumber: big.NewInt(99), BlockHash: creation.BlockHash, TxHash: creation.TxHash, Logs: []*types.Log{&creation, &mint, &increase}}
	f.logs = []types.Log{mint}
	price := new(big.Int).Lsh(big.NewInt(1), 96)
	liquidity := big.NewInt(1000000)
	state := clliquidity.State{SqrtPriceX96: price, Tick: 0, Spacing: 60, ActiveLiquidity: liquidity, Complete: true, Ticks: []clliquidity.Tick{{Index: -60, Gross: liquidity, Net: liquidity}, {Index: 60, Gross: liquidity, Net: new(big.Int).Neg(liquidity)}}}
	req := Request{ChainID: 8453, Protocol: protocol, Creation: creation, Principal: Principal{Pool: pool, Snapshot: clliquidity.Snapshot{BlockNumber: 100, BlockHash: f.hash, BlockTime: time.Unix(1800000000, 0), State: state}}}
	f.integer(factory, get, pool.Big(), token0.Big(), token1.Big(), third)
	f.integer(manager, "factory()", factory.Big())
	f.integer(pool, "factory()", factory.Big())
	slot := make([]*big.Int, slotWords)
	for i := range slot {
		slot[i] = new(big.Int)
	}
	slot[0] = price
	f.put(pool, "slot0()", words(slot...))
	f.integer(pool, "liquidity()", liquidity)
	f.put(manager, "positions(uint256)", words(new(big.Int), new(big.Int), token0.Big(), token1.Big(), third, big.NewInt(-60), big.NewInt(60), liquidity, new(big.Int), new(big.Int), new(big.Int), new(big.Int)), big.NewInt(1))
	packed := append([]byte{}, manager[:]...)
	packed = append(packed, 0xff, 0xff, 0xc4, 0, 0, 60)
	f.put(pool, "positions(bytes32)", words(liquidity, new(big.Int), new(big.Int), new(big.Int), new(big.Int)), crypto.Keccak256Hash(packed).Big())
	owner := common.HexToAddress("0x300001")
	f.integer(manager, "ownerOf(uint256)", owner.Big(), big.NewInt(1))
	f.integer(manager, "getApproved(uint256)", new(big.Int), big.NewInt(1))
	f.responses[callKey(owner, manager, query("decreaseLiquidity((uint256,uint128,uint256,uint256,uint256))", big.NewInt(1), liquidity, new(big.Int), new(big.Int), big.NewInt(1800003600)))] = make([]byte, 64)
	return f, req, manager
}

func limits() Limits {
	return Limits{Timeout: 2 * time.Minute, MaxCalls: 128, MaxReceipts: 64, LogBlockRange: 2000, MaxLogs: 2048, MaxPositions: 256, MaxResponseBytes: 2 << 20}
}
func run(t *testing.T, f *fakeRPC, req Request) (Result, error) {
	t.Helper()
	r, err := NewReader(f, limits())
	if err != nil {
		t.Fatal(err)
	}
	return r.Analyze(context.Background(), req)
}
func runtime(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".hex")
	if err != nil {
		t.Fatal(err)
	}
	code, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// TestReader verifies pool discovery, complete coverage, caching and fail-closed acquisition.
//
// Version:
//   - 2026-09-23: Account for discovery from the creation receipt.
func TestReader(t *testing.T) {
	for _, protocol := range []clliquidity.Protocol{clliquidity.V3, clliquidity.Slipstream} {
		t.Run(string(rune('0'+protocol)), func(t *testing.T) {
			f, req, _ := fixture(t, protocol)
			result, err := run(t, f, req)
			if err != nil || result.Observation == nil || result.Observation.AllPositionsProtected || *result.Observation.Token0.LockedPercentage != "0" || result.Metrics.AdditionalReceipts != 1 || len(result.PositionIDs) != 1 {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			t.Logf("cold calls=%d receipts=%d", result.Metrics.Calls, result.Metrics.AdditionalReceipts)
			req.PositionIDs = result.PositionIDs
			req.Receipts = f.receipts
			f.logs = nil
			result, err = run(t, f, req)
			if err != nil || result.Observation == nil || result.Metrics.AdditionalReceipts != 0 || result.Metrics.Methods["eth_getLogs"] != 0 {
				t.Fatalf("cached result=%+v error=%v", result, err)
			}
		})
	}
	t.Run("unexplained principal", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		f.logs = nil
		f.receipts[req.Creation.TxHash].Logs = f.receipts[req.Creation.TxHash].Logs[:1]
		result, err := run(t, f, req)
		if err != nil || result.Observation != nil || result.Reason != "incomplete_position_coverage" {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("unknown runtime", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		f.codes[common.HexToAddress("0x300001")] = []byte{0x60, 0}
		result, err := run(t, f, req)
		if err != nil || result.Observation != nil || result.Reason != "unsupported_custody" {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("delegated EOA unresolved", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		f.codes[common.HexToAddress("0x300001")] = append([]byte{0xef, 1, 0}, make([]byte, 20)...)
		result, err := run(t, f, req)
		if err != nil || result.Observation != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("block mismatch", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		f.header.Time++
		result, err := run(t, f, req)
		if err == nil || result.Observation != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("mixed principal", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		req.Principal.Snapshot.State.ActiveLiquidity = big.NewInt(123)
		result, err := run(t, f, req)
		if err == nil || result.Observation != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("rpc error no retry", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		sentinel := errors.New("fixture failure")
		f.fail = sentinel
		result, err := run(t, f, req)
		if !errors.Is(err, sentinel) || result.Metrics.Calls != 1 || result.Observation != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("budget boundary", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		lim := limits()
		lim.MaxCalls = 3
		r, err := NewReader(f, lim)
		if err != nil {
			t.Fatal(err)
		}
		result, err := r.Analyze(context.Background(), req)
		if !errors.Is(err, ErrBudget) || result.Metrics.Calls != 3 || result.Observation != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		r, err := NewReader(f, limits())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		result, err := r.Analyze(ctx, req)
		if !errors.Is(err, context.DeadlineExceeded) || result.Metrics.Calls != 0 {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

// TestCustodyModels verifies reviewed runtime, expiry and unresolved escape paths.
//
// Version:
//   - 2026-09-23: Cover operator approvals without changing matched runtime.
func TestCustodyModels(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run("vault expired="+big.NewInt(int64(boolInt(expired))).String(), func(t *testing.T) {
			f, req, manager := fixture(t, clliquidity.V3)
			owner := common.HexToAddress("0x300001")
			f.codes[owner] = runtime(t, "vault")
			fields, ok := match(f.codes[owner], "vault")
			if !ok {
				t.Fatal("fixture mismatch")
			}
			f.integer(owner, "vaultKeyId()", new(big.Int))
			until := int64(1800000100)
			if expired {
				until = 1799999900
			}
			f.integer(owner, "unlockTimestamp()", big.NewInt(until))
			f.integer(owner, "isUnlocked()", new(big.Int))
			beneficiary := common.BigToAddress(fields["beneficiary"])
			f.responses[callKey(beneficiary, owner, query("partialNonFungibleTokenUnlock(address,uint256)", manager.Big(), big.NewInt(1)))] = []byte{}
			result, err := run(t, f, req)
			if err != nil || result.Observation == nil || result.Observation.AllPositionsProtected == expired {
				t.Fatalf("%+v %v", result, err)
			}
			if !expired && (*result.Observation.Token0.LockedPercentage != "100" || *result.Observation.CanWeakenProtection) {
				t.Fatalf("%+v", result.Observation)
			}
		})
	}
	for _, scenario := range []string{"locked", "migration", "approval", "gauge", "modified code", "operator setter code", "active operator"} {
		t.Run(scenario, func(t *testing.T) {
			f, req, manager := fixture(t, clliquidity.Slipstream)
			owner := common.HexToAddress("0x300001")
			implementation := common.HexToAddress("0xfe678bffc3c1c8d1de4478cc5c3e1b93ee4638ae")
			clone := append(common.FromHex("0x363d3d373d3d3d363d73"), implementation[:]...)
			clone = append(clone, common.FromHex("0x5af43d82803e903d91602b57fd5bf3")...)
			f.codes[owner] = clone
			f.codes[implementation] = runtime(t, "clLocker")
			fields, ok := match(f.codes[implementation], "clLocker")
			if !ok {
				t.Fatal("fixture mismatch")
			}
			factory := common.BigToAddress(fields["factory"])
			voter := common.BigToAddress(fields["voter"])
			f.codes[factory] = runtime(t, "clFactory")
			f.integer(owner, "pool()", req.Principal.Pool.Big())
			f.integer(owner, "lp()", big.NewInt(1))
			f.integer(factory, "instances(address)", big.NewInt(1), owner.Big())
			f.integer(owner, "owner()", common.HexToAddress("0x400001").Big())
			f.integer(owner, "lockedUntil()", big.NewInt(4294967295))
			f.integer(owner, "staked()", new(big.Int))
			f.integer(factory, "newLockerFactory()", new(big.Int))
			f.integer(owner, "gauge()", new(big.Int))
			f.integer(voter, "gauges(address)", new(big.Int), req.Principal.Pool.Big())
			f.integer(factory, "owner()", common.HexToAddress("0x500001").Big())
			switch scenario {
			case "migration":
				f.integer(factory, "newLockerFactory()", common.HexToAddress("0x600001").Big())
			case "approval":
				f.integer(manager, "getApproved(uint256)", common.HexToAddress("0x600001").Big(), big.NewInt(1))
			case "gauge":
				f.integer(owner, "gauge()", common.HexToAddress("0x600001").Big())
			case "modified code":
				f.codes[implementation][100] ^= 1
			case "operator setter code":
				f.codes[implementation] = append(f.codes[implementation], crypto.Keccak256([]byte("setApprovalForAll(address,bool)"))[:4]...)
			case "active operator":
				operator := common.HexToAddress("0x700001")
				f.approvalLogs = []types.Log{approvalEvent(manager, owner, operator, 1)}
				f.integer(manager, "isApprovedForAll(address,address)", big.NewInt(1), owner.Big(), operator.Big())
			}
			result, err := run(t, f, req)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "locked" {
				if result.Observation == nil || !result.Observation.AllPositionsProtected || !*result.Observation.CanWeakenProtection || *result.Observation.Token0.LockedPercentage != "100" || *result.Observation.Token0.PermanentlyProtectedPercentage != "0" {
					t.Fatalf("%+v", result)
				}
			} else if result.Observation != nil {
				t.Fatalf("unsafe observation: %+v", result)
			}
		})
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// TestAcquisitionLimits verifies receipt and payload limits, reused evidence and constructor validation.
//
// Version:
//   - 2026-09-23: Separate creation and mint receipts for the acquisition limit.
func TestAcquisitionLimits(t *testing.T) {
	t.Run("constructor", func(t *testing.T) {
		f, _, _ := fixture(t, clliquidity.V3)
		if _, err := NewReader(nil, limits()); err == nil {
			t.Fatal("nil rpc accepted")
		}
		if _, err := NewReader(f, Limits{}); err == nil {
			t.Fatal("zero limits accepted")
		}
	})
	t.Run("receipt budget excludes cache hits", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		creation := f.receipts[req.Creation.TxHash]
		newHash := common.HexToHash("0x33")
		mint, inc := *creation.Logs[1], *creation.Logs[2]
		creation.Logs = creation.Logs[:1]
		mint.TxHash = newHash
		inc.TxHash = newHash
		f.logs = []types.Log{mint}
		f.receipts[newHash] = &types.Receipt{Status: 1, BlockNumber: big.NewInt(99), BlockHash: mint.BlockHash, TxHash: newHash, Logs: []*types.Log{&mint, &inc}}
		lim := limits()
		lim.MaxReceipts = 1
		r, err := NewReader(f, lim)
		if err != nil {
			t.Fatal(err)
		}
		result, err := r.Analyze(context.Background(), req)
		if !errors.Is(err, ErrBudget) || result.Observation != nil || result.Metrics.AdditionalReceipts != 1 {
			t.Fatalf("%+v %v", result, err)
		}
		req.Receipts = map[common.Hash]*types.Receipt{req.Creation.TxHash: creation}
		result, err = r.Analyze(context.Background(), req)
		if err != nil || result.Observation == nil || result.Metrics.AdditionalReceipts != 1 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("cached receipt identity", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		bad := *f.receipts[req.Creation.TxHash]
		bad.BlockHash = common.HexToHash("0xffff")
		req.Receipts = map[common.Hash]*types.Receipt{req.Creation.TxHash: &bad}
		result, err := run(t, f, req)
		if err == nil || result.Observation != nil || result.Metrics.AdditionalReceipts != 0 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("payload budget", func(t *testing.T) {
		f, req, _ := fixture(t, clliquidity.V3)
		lim := limits()
		lim.MaxResponseBytes = 32
		r, err := NewReader(f, lim)
		if err != nil {
			t.Fatal(err)
		}
		result, err := r.Analyze(context.Background(), req)
		if !errors.Is(err, ErrBudget) || result.Observation != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("core coverage", func(t *testing.T) {
		f, req, manager := fixture(t, clliquidity.V3)
		packed := append([]byte{}, manager[:]...)
		packed = append(packed, 0xff, 0xff, 0xc4, 0, 0, 60)
		f.put(req.Principal.Pool, "positions(bytes32)", make([]byte, 160), crypto.Keccak256Hash(packed).Big())
		result, err := run(t, f, req)
		if err != nil || result.Observation != nil || result.Reason != "incomplete_position_coverage" {
			t.Fatalf("%+v %v", result, err)
		}
	})
}
