package lpprotection

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
	"github.com/k4k3ru-hub/onchain/go/venues/uniswap/v4/deployment"
)

func v4Fixture(t *testing.T) (*fakeRPC, Request, deployment.Deployment) {
	t.Helper()
	f, req, _ := fixture(t, clliquidity.V3)
	d, err := deployment.ByChainID(8453)
	if err != nil {
		t.Fatal(err)
	}
	f.responses = make(map[string][]byte)
	creationHeader := types.CopyHeader(f.header)
	creationHeader.Number = big.NewInt(99)
	key := words(new(big.Int), big.NewInt(0x200002), big.NewInt(3000), big.NewInt(60), new(big.Int))
	poolID := crypto.Keccak256Hash(key)
	creation := types.Log{Address: d.PoolManager, Topics: []common.Hash{crypto.Keccak256Hash([]byte("Initialize(bytes32,address,address,uint24,int24,address,uint160,int24)")), poolID, {}, common.BigToHash(big.NewInt(0x200002))}, Data: words(big.NewInt(3000), big.NewInt(60), new(big.Int), req.Principal.Snapshot.State.SqrtPriceX96, new(big.Int)), BlockNumber: 99, BlockHash: creationHeader.Hash(), TxHash: common.HexToHash("0x22")}
	modify := types.Log{Address: d.PoolManager, Topics: []common.Hash{v4ModifyTopic(), poolID, common.BigToHash(d.PositionManager.Big())}, Data: words(big.NewInt(-60), big.NewInt(60), big.NewInt(1000000), big.NewInt(1)), BlockNumber: 99, BlockHash: creation.BlockHash, TxHash: creation.TxHash, Index: 1}
	f.receipts[creation.TxHash] = &types.Receipt{Status: 1, BlockNumber: big.NewInt(99), BlockHash: creation.BlockHash, TxHash: creation.TxHash, Logs: []*types.Log{&creation, &modify}}
	f.logs = []types.Log{modify}
	req.Protocol, req.Creation = clliquidity.V4, creation
	req.Principal.Pool, req.Principal.PoolID = d.PoolManager, poolID
	f.codes[d.PositionManager] = runtime(t, "v4_position_manager")
	f.integer(d.PositionManager, "poolManager()", d.PoolManager.Big())
	f.integer(d.StateView, "poolManager()", d.PoolManager.Big())
	v4PutState(f, req, d)
	v4PutPosition(t, f, req, d, 1, -60, 60, big.NewInt(1000000))
	return f, req, d
}

func v4PutState(f *fakeRPC, req Request, d deployment.Deployment) {
	s := req.Principal.Snapshot.State
	f.put(d.StateView, "getSlot0(bytes32)", words(s.SqrtPriceX96, big.NewInt(int64(s.Tick)), new(big.Int), big.NewInt(3000)), req.Principal.PoolID.Big())
	f.integer(d.StateView, "getLiquidity(bytes32)", s.ActiveLiquidity, req.Principal.PoolID.Big())
}

func v4PoolWords(req Request) []*big.Int {
	return []*big.Int{req.Creation.Topics[2].Big(), req.Creation.Topics[3].Big(), new(big.Int).SetBytes(req.Creation.Data[:32]), new(big.Int).SetBytes(req.Creation.Data[32:64]), new(big.Int).SetBytes(req.Creation.Data[64:96])}
}

func v4PutPosition(t *testing.T, f *fakeRPC, req Request, d deployment.Deployment, id int64, lower, upper int32, liquidity *big.Int) {
	t.Helper()
	n := big.NewInt(id)
	packed := req.Principal.PoolID.Big()
	packed.Rsh(packed, 56).Lsh(packed, 56)
	packed.Or(packed, new(big.Int).Lsh(new(big.Int).SetUint64(uint64(uint32(lower)&0xffffff)), 8))
	packed.Or(packed, new(big.Int).Lsh(new(big.Int).SetUint64(uint64(uint32(upper)&0xffffff)), 32))
	f.put(d.PositionManager, "getPoolAndPositionInfo(uint256)", words(append(v4PoolWords(req), packed)...), n)
	f.integer(d.PositionManager, "getPositionLiquidity(uint256)", liquidity, n)
	f.put(d.StateView, "getPositionInfo(bytes32,address,int24,int24,bytes32)", words(liquidity, new(big.Int), new(big.Int)), req.Principal.PoolID.Big(), d.PositionManager.Big(), big.NewInt(int64(lower)), big.NewInt(int64(upper)), n)
	owner := common.HexToAddress("0x300001")
	f.integer(d.PositionManager, "ownerOf(uint256)", owner.Big(), n)
	f.integer(d.PositionManager, "getApproved(uint256)", new(big.Int), n)
	// Independent go-ethereum ABI encoder, rather than the production encoder.
	decrease := v4ABIPack(t, []string{"uint256", "uint256", "uint128", "uint128", "bytes"}, n, liquidity, new(big.Int), new(big.Int), []byte{})
	take := v4ABIPack(t, []string{"address", "address", "address"}, common.BigToAddress(req.Creation.Topics[2].Big()), common.BigToAddress(req.Creation.Topics[3].Big()), owner)
	unlock := v4ABIPack(t, []string{"bytes", "bytes[]"}, []byte{0x01, 0x11}, [][]byte{decrease, take})
	data := append(crypto.Keccak256([]byte("modifyLiquidities(bytes,uint256)"))[:4], v4ABIPack(t, []string{"bytes", "uint256"}, unlock, big.NewInt(req.Principal.Snapshot.BlockTime.Unix()+3600))...)
	if len(data) != 676 {
		t.Fatalf("withdrawal calldata length = %d", len(data))
	}
	f.responses[callKey(owner, d.PositionManager, data)] = []byte{}
}

func v4ABIPack(t *testing.T, names []string, values ...any) []byte {
	t.Helper()
	var arguments abi.Arguments
	for _, name := range names {
		typ, err := abi.NewType(name, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		arguments = append(arguments, abi.Argument{Type: typ})
	}
	b, err := arguments.Pack(values...)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestV4Reader checks full principal withdrawal, receipt reuse and evidence restoration.
//
// Version:
//   - 2026-09-24: Added.
func TestV4Reader(t *testing.T) {
	f, req, d := v4Fixture(t)
	f.historyErr = errors.New("history must not be requested")
	result, err := run(t, f, req)
	if err != nil || result.Observation == nil || result.Observation.AllPositionsProtected || result.Pool != d.PoolManager || result.PoolID != req.Principal.PoolID || result.Manager != d.PositionManager || len(result.Positions) != 1 || result.Positions[0].Kind != "withdrawable" || result.Positions[0].Model != "v4-direct-eoa-v1" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	for _, token := range []TokenProtection{result.Observation.Token0, result.Observation.Token1} {
		if token.LockedPercentage == nil || *token.LockedPercentage != "0" || token.PermanentlyProtectedPercentage == nil || *token.PermanentlyProtectedPercentage != "0" {
			t.Fatalf("unexpected protection %+v", token)
		}
	}
	if result.Metrics.Methods["eth_getLogs"] != 0 || result.Metrics.AdditionalReceipts != 1 {
		t.Fatalf("metrics=%+v", result.Metrics)
	}
	t.Logf("fixture cold calls=%d receipts=%d", result.Metrics.Calls, result.Metrics.AdditionalReceipts)
	b, err := result.Evidence.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	req.Evidence, err = RestoreEvidence(b, 512<<10)
	if err != nil {
		t.Fatal(err)
	}
	// Reconstruct IDs and ranges from saved receipts alone.
	f.receipts, f.logs = nil, nil
	result, err = run(t, f, req)
	if err != nil || result.Observation == nil || len(result.PositionIDs) != 1 || result.Metrics.AdditionalReceipts != 0 || result.Metrics.Methods["eth_getLogs"] != 0 {
		t.Fatalf("restored=%+v error=%v", result, err)
	}
	t.Logf("fixture warm calls=%d receipts=%d", result.Metrics.Calls, result.Metrics.AdditionalReceipts)
}

// TestV4Identity rejects unsupported deployments and observations with mixed identities.
//
// Version:
//   - 2026-09-24: Added.
func TestV4Identity(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*fakeRPC, *Request, deployment.Deployment)
		wantErr bool
	}{
		{"different pool id with same address suffix", func(_ *fakeRPC, r *Request, _ deployment.Deployment) { r.Principal.PoolID[0] ^= 1 }, true},
		{"wrong pool manager", func(_ *fakeRPC, r *Request, _ deployment.Deployment) {
			r.Principal.Pool = common.HexToAddress("0x100001")
		}, false},
		{"wrong chain", func(f *fakeRPC, _ *Request, _ deployment.Deployment) { f.chain = big.NewInt(1) }, true},
		{"unsupported chain", func(_ *fakeRPC, r *Request, _ deployment.Deployment) { r.ChainID = 1 }, false},
		{"runtime substitution", func(f *fakeRPC, _ *Request, d deployment.Deployment) { f.codes[d.PositionManager][0] ^= 1 }, false},
		{"state view binding", func(f *fakeRPC, _ *Request, d deployment.Deployment) {
			f.integer(d.StateView, "poolManager()", big.NewInt(1))
		}, true},
		{"mixed state", func(f *fakeRPC, r *Request, d deployment.Deployment) {
			f.integer(d.StateView, "getLiquidity(bytes32)", big.NewInt(5), r.Principal.PoolID.Big())
		}, true},
		{"creation reorg", func(_ *fakeRPC, r *Request, _ deployment.Deployment) {
			r.Creation.BlockHash = common.HexToHash("0xabcd")
		}, true},
		{"hook present", func(_ *fakeRPC, r *Request, _ deployment.Deployment) {
			copy(r.Creation.Data[64:96], words(big.NewInt(0x123456)))
			r.Principal.PoolID = crypto.Keccak256Hash(words(v4PoolWords(*r)...))
			r.Creation.Topics[1] = r.Principal.PoolID
		}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, req, d := v4Fixture(t)
			test.mutate(f, &req, d)
			result, err := run(t, f, req)
			if (err != nil) != test.wantErr || result.Observation != nil || result.Reason == "" {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

// TestV4Coverage checks per-salt coverage, out-of-range dust and unsupported managers.
//
// Version:
//   - 2026-09-24: Added.
func TestV4Coverage(t *testing.T) {
	t.Run("same ticks different salt with duplicate ids", func(t *testing.T) {
		f, req, d := v4Fixture(t)
		req.PositionIDs = []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(2)}
		v4PutPosition(t, f, req, d, 1, -60, 60, big.NewInt(400000))
		v4PutPosition(t, f, req, d, 2, -60, 60, big.NewInt(600000))
		result, err := run(t, f, req)
		if err != nil || result.Observation == nil || len(result.Positions) != 2 || len(result.PositionIDs) != 2 || result.Metrics.Methods["eth_getLogs"] != 0 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("core salt mismatch", func(t *testing.T) {
		f, req, d := v4Fixture(t)
		f.put(d.StateView, "getPositionInfo(bytes32,address,int24,int24,bytes32)", words(big.NewInt(999999), new(big.Int), new(big.Int)), req.Principal.PoolID.Big(), d.PositionManager.Big(), big.NewInt(-60), big.NewInt(60), big.NewInt(1))
		result, err := run(t, f, req)
		if err == nil || result.Observation != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("missing out of range dust", func(t *testing.T) {
		f, req, _ := v4Fixture(t)
		req.Principal.Snapshot.State.Ticks = append(req.Principal.Snapshot.State.Ticks, clliquidity.Tick{Index: 120, Gross: big.NewInt(1), Net: big.NewInt(1)}, clliquidity.Tick{Index: 180, Gross: big.NewInt(1), Net: big.NewInt(-1)})
		result, err := run(t, f, req)
		if err != nil || result.Observation != nil || result.Reason != "incomplete_position_coverage" || len(f.queries) != 1 {
			t.Fatalf("%+v %v", result, err)
		}
		q := f.queries[0]
		if len(q.Topics) != 2 || q.Topics[1][0] != req.Principal.PoolID || len(q.Addresses) != 1 || q.Addresses[0] != req.Principal.Pool {
			t.Fatalf("pool-scoped query required: %+v", q)
		}
	})
	t.Run("non official manager", func(t *testing.T) {
		f, req, _ := v4Fixture(t)
		f.receipts[req.Creation.TxHash].Logs[1].Topics[2] = common.HexToHash("0x999999")
		result, err := run(t, f, req)
		if err != nil || result.Observation != nil || result.Reason != "incomplete_position_coverage" || len(result.PositionIDs) != 0 {
			t.Fatalf("%+v %v", result, err)
		}
	})
	t.Run("all liquidity out of range", func(t *testing.T) {
		f, req, d := v4Fixture(t)
		// ceil(2^96 * (10001/10000)^30), the exact tick-60 boundary.
		price, ok := new(big.Int).SetString("79466191966197645195421774833", 10)
		if !ok {
			t.Fatal("invalid fixture price")
		}
		req.Principal.Snapshot.State.SqrtPriceX96 = price
		req.Principal.Snapshot.State.Tick = 60
		req.Principal.Snapshot.State.ActiveLiquidity = new(big.Int)
		v4PutState(f, req, d)
		result, err := run(t, f, req)
		if err != nil || result.Observation == nil || result.Observation.Token0.LockedPercentage != nil || result.Observation.Token1.LockedPercentage == nil || *result.Observation.Token1.LockedPercentage != "0" {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

type v4WithdrawalRPC struct {
	*fakeRPC
	withdrawalErr      error
	getterErr          error
	withdrawals        int
	observationHeaders int
	reorgAtFinish      bool
}

// HeaderByNumber injects a final observation reorg after successful state reads.
//
// Version:
//   - 2026-09-24: Added.
func (f *v4WithdrawalRPC) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	h, err := f.fakeRPC.HeaderByNumber(ctx, number)
	if number.Cmp(f.header.Number) == 0 {
		f.observationHeaders++
		if f.reorgAtFinish && f.observationHeaders == 2 {
			h = types.CopyHeader(h)
			h.Extra = []byte("replacement")
		}
	}
	return h, err
}

// CallContractAtHash injects withdrawal/getter failure without changing other reads.
//
// Version:
//   - 2026-09-24: Added.
func (f *v4WithdrawalRPC) CallContractAtHash(ctx context.Context, msg ethereum.CallMsg, hash common.Hash) ([]byte, error) {
	if len(msg.Data) >= 4 && bytes.Equal(msg.Data[:4], query("modifyLiquidities(bytes,uint256)")) {
		f.withdrawals++
		if f.withdrawalErr != nil {
			return nil, f.withdrawalErr
		}
	}
	if f.getterErr != nil && len(msg.Data) >= 4 && bytes.Equal(msg.Data[:4], query("getPoolAndPositionInfo(uint256)")) {
		return nil, f.getterErr
	}
	return f.fakeRPC.CallContractAtHash(ctx, msg, hash)
}

// TestV4Custody distinguishes successful withdrawal from reverts and unknown code.
//
// Version:
//   - 2026-09-24: Added.
func TestV4Custody(t *testing.T) {
	sentinel := errors.New("withdrawal reverted")
	for _, test := range []struct {
		name      string
		code      []byte
		fail      error
		wantCalls int
		wantErr   bool
	}{
		{"ordinary owner", nil, nil, 1, false},
		{"reverted owner", nil, sentinel, 1, true},
		{"unknown contract", []byte{0x60, 0}, nil, 0, false},
		{"delegated account", append([]byte{0xef, 1, 0}, make([]byte, 20)...), nil, 0, false},
		{"v3 custody template is not a v4 rule", runtime(t, "vault"), nil, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, req, _ := v4Fixture(t)
			f.codes[common.HexToAddress("0x300001")] = test.code
			wrapped := &v4WithdrawalRPC{fakeRPC: f, withdrawalErr: test.fail}
			r, err := NewReader(wrapped, limits())
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.Analyze(context.Background(), req)
			if (err != nil) != test.wantErr || wrapped.withdrawals != test.wantCalls || test.fail != nil && !errors.Is(err, test.fail) {
				t.Fatalf("%+v %v withdrawals=%d", result, err, wrapped.withdrawals)
			}
			if test.name != "ordinary owner" && result.Observation != nil {
				t.Fatal("unknown or reverted custody published percentages")
			}
		})
	}
}

// TestV4MalformedPosition rejects inconsistent current NFT metadata before publishing values.
//
// Version:
//   - 2026-09-24: Added.
func TestV4MalformedPosition(t *testing.T) {
	for _, mode := range []string{"other pool", "packed pool prefix", "non-aligned tick", "unknown subscriber flag", "truncated tuple", "liquidity overflow"} {
		t.Run(mode, func(t *testing.T) {
			f, req, d := v4Fixture(t)
			key := callKey(common.Address{}, d.PositionManager, query("getPoolAndPositionInfo(uint256)", big.NewInt(1)))
			data := f.responses[key]
			switch mode {
			case "other pool":
				data[63] ^= 1
			case "packed pool prefix":
				data[160] ^= 1
			case "non-aligned tick":
				data[190] ^= 1
			case "unknown subscriber flag":
				data[191] = 2
			case "truncated tuple":
				f.responses[key] = data[:160]
			case "liquidity overflow":
				f.integer(d.PositionManager, "getPositionLiquidity(uint256)", new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
			}
			result, err := run(t, f, req)
			if err == nil || result.Observation != nil {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

// TestV4Reorg discards evidence and observations when the final canonical hash changes.
//
// Version:
//   - 2026-09-24: Added.
func TestV4Reorg(t *testing.T) {
	f, req, _ := v4Fixture(t)
	r, err := NewReader(&v4WithdrawalRPC{fakeRPC: f, reorgAtFinish: true}, limits())
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.Analyze(context.Background(), req)
	if !errors.Is(err, ErrReorg) || result.Observation != nil || result.Reason != "observation_reorg" || result.Evidence == nil || len(result.Evidence.data.Acquired) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
}
