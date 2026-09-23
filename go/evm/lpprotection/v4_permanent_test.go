package lpprotection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

func permanentSample(t *testing.T) (*creationFake, Request) {
	t.Helper()
	b, err := os.ReadFile("testdata/v4-permanent-rpc.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &creationFake{}
	if err := json.Unmarshal(b, &f.data); err != nil {
		t.Fatal(err)
	}
	var input struct{ Creation types.Log }
	if err := json.Unmarshal(b, &input); err != nil {
		t.Fatal(err)
	}
	h := f.data.Headers[51604306]
	manager := common.HexToAddress("0x7c5f5a4bbd8fd63184577525326123b519429bdc")
	f.data.Codes[strings.ToLower(manager.Hex()+h.Hash().Hex())] = "0x" + common.Bytes2Hex(runtime(t, "v4_position_manager"))
	price, ok := new(big.Int).SetString("799603176048022585010545010154905", 10)
	if !ok {
		t.Fatal("invalid fixture price")
	}
	liquidity, ok := new(big.Int).SetString("99084351947979319780625", 10)
	if !ok {
		t.Fatal("invalid fixture liquidity")
	}
	s := clliquidity.State{Complete: true, SqrtPriceX96: price, Tick: 184400, Spacing: 200, ActiveLiquidity: new(big.Int), Ticks: []clliquidity.Tick{
		{Index: -887200, Gross: new(big.Int).Set(liquidity), Net: new(big.Int).Set(liquidity)},
		{Index: 184400, Gross: new(big.Int).Set(liquidity), Net: new(big.Int).Neg(liquidity)},
	}}
	req := Request{ChainID: 8453, Protocol: clliquidity.V4, Creation: input.Creation,
		Principal:     Principal{Pool: input.Creation.Address, PoolID: input.Creation.Topics[1], Snapshot: clliquidity.Snapshot{BlockNumber: h.Number.Uint64(), BlockHash: h.Hash(), BlockTime: time.Unix(int64(h.Time), 0).UTC(), State: s}},
		CreationHints: []CreationHint{{Factory: common.HexToAddress("0x815542e8b392389a1389e22e588e4b62a67ade72"), TransactionHash: common.HexToHash("0xf58f481f38a00ec6db604d14a9b79c711fa4aac74dc4577072376eeecdac8a60")}},
	}
	return f, req
}

func permanentRun(t *testing.T, f *creationFake, req Request) (Result, error) {
	t.Helper()
	r, err := NewReaderWithCreationRPC(f, f, limits())
	if err != nil {
		t.Fatal(err)
	}
	before := f.attempts
	result, err := r.Analyze(context.Background(), req)
	if result.Metrics.Calls != f.attempts-before {
		t.Fatalf("unaccounted calls: %+v actual=%d", result.Metrics, f.attempts-before)
	}
	return result, err
}

func permanentCallKey(req Request, address common.Address, signature string, args ...*big.Int) string {
	return strings.ToLower(common.Address{}.Hex() + address.Hex() + "0x" + common.Bytes2Hex(query(signature, args...)) + req.Principal.Snapshot.BlockHash.Hex())
}

// TestV4PermanentRecordedSample runs the production reader against the recorded historical pool.
//
// Version:
//   - 2026-09-24: Added.
func TestV4PermanentRecordedSample(t *testing.T) {
	f, req := permanentSample(t)
	result, err := permanentRun(t, f, req)
	if err != nil || result.Observation == nil || !result.Observation.AllPositionsProtected || len(result.Positions) != 1 || result.Positions[0].Kind != "permanent" {
		t.Fatalf("%+v %v", result, err)
	}
	v := result.Observation
	if v.Token0.LockedPercentage != nil || v.Token0.PermanentlyProtectedPercentage != nil || *v.Token1.LockedPercentage != "0" || *v.Token1.PermanentlyProtectedPercentage != "100" || v.EarliestUnlockAt != nil || v.CanWeakenProtection == nil || *v.CanWeakenProtection {
		t.Fatalf("%+v", v)
	}
	proofs := result.Evidence.PermanentCustodies()
	if len(proofs) != 1 || proofs[0].Rule != permanentRule || proofs[0].FactoryTransaction != req.CreationHints[0].TransactionHash || len(result.Evidence.Histories()) != 0 || len(f.queries) != 0 {
		t.Fatalf("unexpected proof/history: %+v", proofs)
	}
	proofs[0].Rule = "mutated copy"
	if result.Evidence.PermanentCustodies()[0].Rule != permanentRule {
		t.Fatal("mutable proof accessor")
	}
	b, err := result.Evidence.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 512<<10 {
		t.Fatalf("evidence exceeds pool persistence limit: %d", len(b))
	}
	t.Logf("recorded fixture cold calls=%d receipts=%d bytes=%d evidence_bytes=%d", result.Metrics.Calls, result.Metrics.AdditionalReceipts, result.Metrics.ResponseBytes, len(b))
	req.Evidence, err = RestoreEvidence(b, 512<<10)
	if err != nil {
		t.Fatal(err)
	}
	req.CreationHints = nil
	// Immutable acquisition must now come from the internal evidence cache.
	f.data.Transactions = nil
	f.data.Receipts = nil
	result, err = permanentRun(t, f, req)
	if err != nil || result.Observation == nil || !result.Observation.AllPositionsProtected || result.Metrics.AdditionalReceipts != 0 || result.Metrics.Methods["eth_getTransactionByHash"] != 0 || len(f.queries) != 0 {
		t.Fatalf("warm=%+v %v", result, err)
	}
	t.Logf("recorded fixture warm calls=%d receipts=%d bytes=%d", result.Metrics.Calls, result.Metrics.AdditionalReceipts, result.Metrics.ResponseBytes)
}

// TestV4PermanentFailures prevents runtime-only or incomplete creation evidence from becoming protection.
//
// Version:
//   - 2026-09-24: Added.
func TestV4PermanentFailures(t *testing.T) {
	for _, mode := range []string{"missing hint", "wrong transaction", "factory code", "locker code", "inconsistent immutable", "manager immutable", "permit2 immutable", "birth runtime", "receipt address", "missing code", "missing transaction", "missing creation dependency"} {
		t.Run(mode, func(t *testing.T) {
			f, req := permanentSample(t)
			factory := req.CreationHints[0].Factory
			locker := crypto.CreateAddress(factory, 1)
			mutate := func(address common.Address, model, field string) {
				key := strings.ToLower(address.Hex() + req.Principal.Snapshot.BlockHash.Hex())
				code := common.FromHex(f.data.Codes[key])
				if field == "" {
					code[100] ^= 1
				} else {
					for _, offset := range template(model).fields[field] {
						code[offset+31] ^= 1
					}
				}
				f.data.Codes[key] = "0x" + common.Bytes2Hex(code)
			}
			switch mode {
			case "missing hint":
				req.CreationHints = nil
			case "wrong transaction":
				req.CreationHints[0].TransactionHash = common.HexToHash("0x123")
			case "factory code":
				mutate(factory, "v4LaunchFactory", "")
			case "locker code":
				mutate(locker, "v4LaunchLocker", "")
			case "inconsistent immutable":
				key := strings.ToLower(locker.Hex() + req.Principal.Snapshot.BlockHash.Hex())
				code := common.FromHex(f.data.Codes[key])
				code[template("v4LaunchLocker").fields["manager"][0]+31] ^= 1
				f.data.Codes[key] = "0x" + common.Bytes2Hex(code)
			case "manager immutable":
				mutate(locker, "v4LaunchLocker", "manager")
			case "permit2 immutable":
				mutate(factory, "v4LaunchFactory", "permit2")
			case "birth runtime":
				f.data.Codes[strings.ToLower(factory.Hex()+f.data.Headers[50940130].Hash().Hex())] = "0x6000"
			case "receipt address":
				f.data.Receipts[req.CreationHints[0].TransactionHash].ContractAddress = common.HexToAddress("0x123")
			case "missing code":
				delete(f.data.Codes, strings.ToLower(factory.Hex()+f.data.Headers[50940130].Hash().Hex()))
			case "missing transaction":
				f.data.Transactions = nil
			}
			var result Result
			var err error
			if mode == "missing creation dependency" {
				r, createErr := NewReader(f, limits())
				if createErr != nil {
					t.Fatal(createErr)
				}
				result, err = r.Analyze(context.Background(), req)
			} else {
				result, err = permanentRun(t, f, req)
			}
			if result.Observation != nil || result.Reason == "" || len(f.queries) != 0 {
				t.Fatalf("%+v %v", result, err)
			}
			if result.Evidence != nil && len(result.Evidence.PermanentCustodies()) != 0 {
				t.Fatal("invalid proof retained")
			}
		})
	}
}

// TestLaunchFactoryDeployment rejects altered constructors even when signed by the correct creating EOA.
//
// Version:
//   - 2026-09-24: Added.
func TestLaunchFactoryDeployment(t *testing.T) {
	f, req := permanentSample(t)
	init := f.data.Transactions[req.CreationHints[0].TransactionHash].Data()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	origin := crypto.PubkeyToAddress(key.PublicKey)
	factory := crypto.CreateAddress(origin, 0)
	manager := common.HexToAddress("0x7c5f5a4bbd8fd63184577525326123b519429bdc")
	permit2 := common.HexToAddress("0x000000000022d473030f116ddee9f6b43ac78ba3")
	for _, mode := range []string{"valid", "prefix changed", "extra suffix", "wrong manager", "wrong permit2", "nonce", "value", "to", "chain", "invalid signature"} {
		t.Run(mode, func(t *testing.T) {
			data := bytes.Clone(init)
			body := &types.DynamicFeeTx{ChainID: big.NewInt(8453), Gas: 20000000, GasFeeCap: big.NewInt(1), GasTipCap: new(big.Int), Value: new(big.Int)}
			switch mode {
			case "prefix changed":
				data[0] ^= 1
			case "extra suffix":
				data = append(data, 0)
			case "wrong manager":
				data[len(data)-33] ^= 1
			case "wrong permit2":
				data[len(data)-1] ^= 1
			case "nonce":
				body.Nonce = 1
			case "value":
				body.Value = big.NewInt(1)
			case "to":
				body.To = &manager
			case "chain":
				body.ChainID = big.NewInt(1)
			}
			body.Data = data
			tx := types.NewTx(body)
			if mode != "invalid signature" {
				var err error
				tx, err = types.SignTx(tx, types.LatestSignerForChainID(body.ChainID), key)
				if err != nil {
					t.Fatal(err)
				}
			}
			sender, ok := launchFactoryDeployment(tx, factory, req.Principal.Pool, manager, permit2)
			if ok != (mode == "valid") || ok && sender != origin {
				t.Fatalf("sender=%s accepted=%t", sender, ok)
			}
		})
	}
}

// TestPermanentAuthorityConflict persists observed approval contradictions across clean reevaluations.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentAuthorityConflict(t *testing.T) {
	f, req := permanentSample(t)
	manager := common.HexToAddress("0x7c5f5a4bbd8fd63184577525326123b519429bdc")
	key := permanentCallKey(req, manager, "getApproved(uint256)", big.NewInt(3073314))
	f.data.Calls[key] = "0x" + common.Bytes2Hex(words(big.NewInt(0x123456)))
	result, err := permanentRun(t, f, req)
	if err != nil || result.Observation != nil || len(result.Evidence.data.PermanentConflicts) != 1 || result.Positions[0].Reason != "authority_conflict" {
		t.Fatalf("%+v %v", result, err)
	}
	b, err := result.Evidence.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	req.Evidence, err = RestoreEvidence(b, 512<<10)
	if err != nil {
		t.Fatal(err)
	}
	f.data.Calls[key] = "0x" + common.Bytes2Hex(words(new(big.Int)))
	result, err = permanentRun(t, f, req)
	if err != nil || result.Observation != nil || result.Positions[0].Reason != "authority_conflict" {
		t.Fatalf("conflict erased: %+v %v", result, err)
	}
}

// TestV4PermanentReorg removes positive creation evidence when a required historical anchor changes.
//
// Version:
//   - 2026-09-24: Added.
func TestV4PermanentReorg(t *testing.T) {
	f, req := permanentSample(t)
	result, err := permanentRun(t, f, req)
	if err != nil || result.Observation == nil {
		t.Fatalf("%+v %v", result, err)
	}
	req.Evidence = result.Evidence
	f.reorgNumber = 50940130
	result, err = permanentRun(t, f, req)
	if !errors.Is(err, ErrReorg) || result.Observation != nil || len(result.Evidence.PermanentCustodies()) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
}
