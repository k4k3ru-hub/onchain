package lpprotection

import (
	"context"
	"encoding/json"
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

type creationFixture struct {
	Headers              map[uint64]*types.Header
	Codes, Nonces, Calls map[string]string
	Transactions         map[common.Hash]*types.Transaction
	Receipts             map[common.Hash]*types.Receipt
}

type creationFake struct {
	data            creationFixture
	queries         []ethereum.FilterQuery
	attempts        int
	failFrom        uint64
	approvals       []types.Log
	currentApproval int64
	reorgNumber     uint64
}

func creationSample(t *testing.T) (*creationFake, Request) {
	t.Helper()
	b, err := os.ReadFile("testdata/creation-rpc.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &creationFake{}
	if err := json.Unmarshal(b, &f.data); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Pool     common.Address
		Creation types.Log
	}
	b, err = os.ReadFile("testdata/live-pools.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	row := rows[1]
	h := f.data.Headers[51596896]
	v, err := f.CallContractAtHash(context.Background(), ethereum.CallMsg{To: &row.Pool, Data: query("slot0()")}, h.Hash())
	if err != nil {
		t.Fatal(err)
	}
	liq := big.NewInt(3174477384325080)
	state := clliquidity.State{Complete: true, SqrtPriceX96: new(big.Int).SetBytes(v[:32]), Tick: int32(signed(new(big.Int).SetBytes(v[32:64])).Int64()), Spacing: 200, ActiveLiquidity: new(big.Int), Ticks: []clliquidity.Tick{{Index: -887200, Gross: new(big.Int).Set(liq), Net: new(big.Int).Set(liq)}, {Index: 253200, Gross: new(big.Int).Set(liq), Net: new(big.Int).Neg(liq)}}}
	req := Request{ChainID: 8453, Protocol: clliquidity.Slipstream, Creation: row.Creation, Principal: Principal{Pool: row.Pool, Snapshot: clliquidity.Snapshot{BlockNumber: 51596896, BlockHash: h.Hash(), BlockTime: time.Unix(int64(h.Time), 0).UTC(), State: state}}, CreationHints: []CreationHint{{Factory: common.HexToAddress("0x932d0b4c00a2a33ef1ec5fe0aa981bd1a00a6f5c"), TransactionHash: common.HexToHash("0xb498956df7c6d233f510db3298adba98aab1d13f7b70079e6324af699ac93701")}}}
	// Supplement fixtures only for calls absent from the earlier research run:
	// deterministic intermediate headers, a stand-in manager runtime (whose hash
	// only namespaces history), and the reviewed implementation at clone birth.
	// The live test separately acquires these from the actual RPC endpoint.
	for n := uint64(51591852); n < 51596896; n += 1000 {
		f.data.Headers[n] = &types.Header{Number: new(big.Int).SetUint64(n), Time: h.Time}
	}
	manager := common.HexToAddress("0xe1f8cd9ac4e4a65f54f38a5cdafca44f6dd68b53")
	f.data.Codes[strings.ToLower(manager.Hex()+h.Hash().Hex())] = "0x6000"
	impl := common.HexToAddress("0xfe678bffc3c1c8d1de4478cc5c3e1b93ee4638ae")
	f.data.Codes[strings.ToLower(impl.Hex()+req.Creation.BlockHash.Hex())] = f.data.Codes[strings.ToLower(impl.Hex()+h.Hash().Hex())]
	f.attempts = 0
	return f, req
}

// ChainID supplies the archived Base identity.
//
// Version:
//   - 2026-09-23: Added.
func (f *creationFake) ChainID(context.Context) (*big.Int, error) {
	f.attempts++
	return big.NewInt(8453), nil
}

// HeaderByNumber supplies canonical or intentionally changed fixture headers.
//
// Version:
//   - 2026-09-23: Added.
func (f *creationFake) HeaderByNumber(_ context.Context, n *big.Int) (*types.Header, error) {
	f.attempts++
	h := f.data.Headers[n.Uint64()]
	if h == nil {
		return nil, errors.New("missing fixture header")
	}
	if f.reorgNumber == n.Uint64() {
		h = types.CopyHeader(h)
		h.Extra = []byte{1}
	}
	return h, nil
}

// CodeAtHash supplies archived runtime bytes at an exact hash.
//
// Version:
//   - 2026-09-23: Added.
func (f *creationFake) CodeAtHash(_ context.Context, a common.Address, h common.Hash) ([]byte, error) {
	f.attempts++
	v, ok := f.data.Codes[strings.ToLower(a.Hex()+h.Hex())]
	if !ok {
		return nil, errors.New("missing fixture code")
	}
	return common.FromHex(v), nil
}

// NonceAtHash supplies the fixture CREATE nonce range.
//
// Version:
//   - 2026-09-23: Added.
func (f *creationFake) NonceAtHash(_ context.Context, a common.Address, h common.Hash) (uint64, error) {
	f.attempts++
	v, ok := f.data.Nonces[strings.ToLower(a.Hex()+h.Hex())]
	if !ok {
		return 0, errors.New("missing fixture nonce")
	}
	n, ok := new(big.Int).SetString(strings.TrimPrefix(v, "0x"), 16)
	if !ok {
		return 0, errors.New("invalid fixture nonce")
	}
	return n.Uint64(), nil
}

// TransactionByHash supplies the signed deployment candidate.
//
// Version:
//   - 2026-09-23: Added.
func (f *creationFake) TransactionByHash(_ context.Context, h common.Hash) (*types.Transaction, bool, error) {
	f.attempts++
	return f.data.Transactions[h], false, nil
}

// TransactionReceipt supplies the actual creation and lock receipts.
//
// Version:
//   - 2026-09-23: Added.
func (f *creationFake) TransactionReceipt(_ context.Context, h common.Hash) (*types.Receipt, error) {
	f.attempts++
	return f.data.Receipts[h], nil
}

// CallContractAtHash supplies archived mutable state or operator test state.
//
// Version:
//   - 2026-09-23: Added.
func (f *creationFake) CallContractAtHash(_ context.Context, m ethereum.CallMsg, h common.Hash) ([]byte, error) {
	f.attempts++
	if len(m.Data) >= 4 && string(m.Data[:4]) == string(query("isApprovedForAll(address,address)")[:4]) {
		return words(big.NewInt(f.currentApproval)), nil
	}
	key := strings.ToLower(m.From.Hex() + m.To.Hex() + "0x" + common.Bytes2Hex(m.Data) + h.Hex())
	v, ok := f.data.Calls[key]
	if !ok {
		return nil, errors.New("missing fixture call: " + key)
	}
	return common.FromHex(v), nil
}

// FilterLogs requires a bounded, owner-filtered history query including birth.
//
// Version:
//   - 2026-09-23: Added.
func (f *creationFake) FilterLogs(_ context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	f.attempts++
	f.queries = append(f.queries, q)
	if q.FromBlock.Uint64() < 51590853 || q.ToBlock.Uint64()-q.FromBlock.Uint64() >= 1000 || len(q.Addresses) != 1 || len(q.Topics) != 2 || q.Topics[0][0] != crypto.Keccak256Hash([]byte("ApprovalForAll(address,address,bool)")) {
		return nil, errors.New("invalid fixture history query")
	}
	if f.failFrom != 0 && q.FromBlock.Uint64() == f.failFrom {
		return nil, errors.New("fixture acquisition failure")
	}
	var logs []types.Log
	for _, l := range f.approvals {
		if l.BlockNumber >= q.FromBlock.Uint64() && l.BlockNumber <= q.ToBlock.Uint64() {
			logs = append(logs, l)
		}
	}
	return logs, nil
}

func analyzeCreation(t *testing.T, f *creationFake, req Request) (Result, error) {
	t.Helper()
	lim := limits()
	lim.LogBlockRange = 1000
	r, err := NewReaderWithCreationRPC(f, f, lim)
	if err != nil {
		t.Fatal(err)
	}
	before := f.attempts
	result, err := r.Analyze(context.Background(), req)
	if result.Metrics.Calls != f.attempts-before {
		t.Fatalf("attempt accounting: %+v actual=%d", result.Metrics, f.attempts-before)
	}
	return result, err
}

// TestCreationRule computes protection through the production rule and checks
// detached evidence, internal persistence, and history reuse without adapters.
//
// Version:
//   - 2026-09-23: Added.
func TestCreationRule(t *testing.T) {
	f, req := creationSample(t)
	result, err := analyzeCreation(t, f, req)
	if err != nil || result.Observation == nil || !result.Observation.AllPositionsProtected {
		t.Fatalf("%+v %v", result, err)
	}
	if result.Observation.Token0.LockedPercentage != nil || *result.Observation.Token1.LockedPercentage != "100" || !*result.Observation.CanWeakenProtection || result.Observation.EarliestUnlockAt.Unix() != 4294967295 {
		t.Fatal("wrong protection semantics")
	}
	if len(f.queries) != 7 || f.queries[0].FromBlock.Uint64() != 51590853 {
		t.Fatal("birth-inclusive coverage missing")
	}
	if len(result.Evidence.Creations()) != 1 || len(result.Evidence.Histories()) != 1 {
		t.Fatal("missing evidence")
	}
	t.Logf("cold=%+v", result.Metrics)
	b, err := json.Marshal(result.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	req.Evidence, err = RestoreEvidence(b, limits().MaxResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	// Mutation of public copies must never change the reusable proof.
	hs := req.Evidence.Histories()
	hs[0].Through.Number = 0
	cs := req.Evidence.Creations()
	cs[0].LockEvent.Data[0] ^= 1
	f.queries = nil
	req.CreationHints = nil
	again, err := analyzeCreation(t, f, req)
	if err != nil || again.Observation == nil || len(f.queries) != 0 || again.Metrics.Methods["eth_getTransactionByHash"] != 0 || again.Metrics.Methods["eth_getTransactionCount"] != 0 {
		t.Fatalf("reuse failed: %+v %v", again, err)
	}
	t.Logf("warm=%+v evidence_bytes=%d", again.Metrics, len(b))
}

// TestCreationAdversarial rejects superficially matching but unproved creation.
//
// Version:
//   - 2026-09-23: Added.
func TestCreationAdversarial(t *testing.T) {
	for _, name := range []string{"missing hint", "unknown deployer code", "wrong birth nonce", "counterfeit runtime", "fake event", "wrong transaction", "unbounded nonce range"} {
		t.Run(name, func(t *testing.T) {
			f, req := creationSample(t)
			switch name {
			case "missing hint":
				req.CreationHints = nil
			case "unknown deployer code":
				for k := range f.data.Codes {
					if strings.HasPrefix(k, "0xba5ed099") {
						f.data.Codes[k] = "0x6000"
					}
				}
			case "wrong birth nonce":
				for k := range f.data.Nonces {
					f.data.Nonces[k] = "0x57"
				}
			case "counterfeit runtime":
				for k := range f.data.Codes {
					if strings.HasPrefix(k, "0x932d") && strings.HasSuffix(k, f.data.Headers[44515546].Hash().Hex()) {
						f.data.Codes[k] = "0x6000"
					}
				}
			case "fake event":
				r := f.data.Receipts[req.Creation.TxHash]
				for _, l := range r.Logs {
					if len(l.Topics) == 3 && l.Topics[0] == crypto.Keccak256Hash([]byte("LockCreated(address,address,uint256,uint32,address,uint16,uint16)")) {
						l.Data[0] ^= 1
					}
				}
			case "wrong transaction":
				hash := req.CreationHints[0].TransactionHash
				f.data.Transactions[hash] = types.NewTx(&types.LegacyTx{})
			case "unbounded nonce range":
				for k := range f.data.Nonces {
					if strings.HasSuffix(k, req.Creation.BlockHash.Hex()) {
						f.data.Nonces[k] = "0xffff"
					}
				}
			}
			result, _ := analyzeCreation(t, f, req)
			if result.Observation != nil || len(result.Evidence.Creations()) != 0 || len(f.queries) != 0 {
				t.Fatalf("accepted unproven birth: %+v", result)
			}
		})
	}
}
