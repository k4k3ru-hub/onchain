//go:build lp_v4_research

package v4proof

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	evmruntime "github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/k4k3ru-hub/onchain/go/evm/clliquidity"
)

var (
	factory = common.HexToAddress("0x815542e8b392389a1389e22e588e4b62a67ade72")
	locker  = common.HexToAddress("0xcd1680d26922fcd9cabfbb8a56ba40c333fd842a")
	manager = common.HexToAddress("0x7c5f5a4bbd8fd63184577525326123b519429bdc")
	core    = common.HexToAddress("0x498581ff718922c3f8e6a244956af099b2652b2b")
	pool    = common.HexToHash("0x2f20eec5945b32624fb6ccb8ba8716aabea99bc7d8e8590b5a863c283042601a")
)

type record struct {
	Method string
	Params json.RawMessage
	Result json.RawMessage
}

type compiled struct {
	Compiler string
	Contract struct {
		ABI json.RawMessage
		EVM struct {
			Bytecode         struct{ Object string }
			DeployedBytecode struct {
				Object              string
				ImmutableReferences map[string][]struct{ Start, Length int }
			}
		}
	}
}

func directory() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func load(t *testing.T, name string, value any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(directory(), name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, value); err != nil {
		t.Fatal(err)
	}
}

func decode[T any](t *testing.T, r map[string]record, name string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(r[name].Result, &value); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return value
}

func records(t *testing.T) map[string]record {
	t.Helper()
	var data struct{ Results map[string]record }
	load(t, "cold-rpc.json", &data)
	return data.Results
}

func chainConfig(t *testing.T) *evmruntime.Config {
	t.Helper()
	s, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	chain := *params.AllDevChainProtocolChanges
	chain.ChainID = big.NewInt(8453)
	return &evmruntime.Config{State: s, ChainConfig: &chain, GasLimit: 20000000, BlockNumber: big.NewInt(50940130), Time: 1788669607}
}

func matchesRuntime(c compiled, actual []byte) bool {
	expected := common.FromHex(c.Contract.EVM.DeployedBytecode.Object)
	if len(expected) != len(actual) {
		return false
	}
	normalized := bytes.Clone(actual)
	for _, sites := range c.Contract.EVM.DeployedBytecode.ImmutableReferences {
		var prior []byte
		for _, site := range sites {
			if site.Start < 0 || site.Length != 32 || site.Start+site.Length > len(actual) {
				return false
			}
			current := actual[site.Start : site.Start+site.Length]
			if prior != nil && !bytes.Equal(prior, current) {
				return false
			}
			prior = current
			clear(normalized[site.Start : site.Start+site.Length])
		}
	}
	return bytes.Equal(expected, normalized)
}

func matchesInit(c compiled, data []byte, args []byte) bool {
	code := common.FromHex(c.Contract.EVM.Bytecode.Object)
	return len(data) == len(code)+len(args) && bytes.Equal(data[:len(code)], code) && bytes.Equal(data[len(code):], args)
}

func encode(t *testing.T, c compiled, name string, args ...any) []byte {
	t.Helper()
	a, err := abi.JSON(bytes.NewReader(c.Contract.ABI))
	if err != nil {
		t.Fatal(err)
	}
	b, err := a.Pack(name, args...)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestCreationProof binds the actual signed first deployment to both compiled runtimes.
//
// Version:
//   - 2026-09-23: Added.
func TestCreationProof(t *testing.T) {
	r := records(t)
	if decode[string](t, r, "chain") != "0x2105" {
		t.Fatal("unexpected chain")
	}
	tx := decode[types.Transaction](t, r, "creation_tx")
	receipt := decode[types.Receipt](t, r, "creation_receipt")
	header := decode[types.Header](t, r, "birth_header")
	for _, name := range []string{"birth", "pool", "observation"} {
		before, after := decode[types.Header](t, r, name+"_header"), decode[types.Header](t, r, name+"_header_final")
		if before.Hash() != after.Hash() {
			t.Fatal("canonical header changed", name)
		}
	}
	origin, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8453)), &tx)
	if err != nil || tx.ChainId().Cmp(big.NewInt(8453)) != 0 || tx.To() != nil || tx.Nonce() != 0 || tx.Value().Sign() != 0 || crypto.CreateAddress(origin, 0) != factory {
		t.Fatal("unsupported first-deployment path", err)
	}
	if receipt.Status != 1 || receipt.TxHash != tx.Hash() || receipt.ContractAddress != factory || receipt.BlockHash != header.Hash() || receipt.BlockNumber.Cmp(header.Number) != 0 {
		t.Fatal("creation receipt mismatch")
	}
	var fc, lc compiled
	load(t, "LaunchFactory-compiled.json", &fc)
	load(t, "LaunchLocker-compiled.json", &lc)
	args := encode(t, fc, "", core, manager, common.HexToAddress("0x000000000022d473030f116ddee9f6b43ac78ba3"))
	if !matchesInit(fc, tx.Data(), args) {
		t.Fatal("constructor does not match the independently compiled model")
	}
	actualFactory := common.FromHex(decode[string](t, r, "factory_code"))
	actualLocker := common.FromHex(decode[string](t, r, "locker_code"))
	if !matchesRuntime(fc, actualFactory) || !matchesRuntime(lc, actualLocker) || decode[string](t, r, "factory_birth_code") != decode[string](t, r, "factory_code") || decode[string](t, r, "locker_birth_code") != decode[string](t, r, "locker_code") {
		t.Fatal("runtime identity differs")
	}
	cfg := chainConfig(t)
	cfg.Origin, cfg.BlockNumber, cfg.Time = origin, header.Number, header.Time
	var trace []string
	cfg.EVMConfig.Tracer = &tracing.Hooks{OnEnter: func(depth int, typ byte, from, to common.Address, input []byte, gas uint64, value *big.Int) {
		trace = append(trace, fmt.Sprintf("%d %s %s -> %s", depth, vm.OpCode(typ), from, to))
		switch {
		case depth == 0 && typ == byte(vm.CREATE) && from == origin && to == factory && bytes.Equal(input, tx.Data()):
		case depth == 1 && typ == byte(vm.CREATE) && from == factory && to == locker && matchesInit(lc, input, common.LeftPadBytes(manager[:], 32)):
		default:
			t.Fatalf("unexpected constructor interaction: %s", trace[len(trace)-1])
		}
	}}
	_, created, _, err := evmruntime.Create(tx.Data(), cfg)
	if err != nil || created != factory || len(trace) != 2 || crypto.CreateAddress(factory, 1) != locker || !bytes.Equal(cfg.State.GetCode(factory), actualFactory) || !bytes.Equal(cfg.State.GetCode(locker), actualLocker) || cfg.State.GetNonce(factory) != 2 || cfg.State.GetNonce(locker) != 1 {
		t.Fatalf("constructor replay differs: err=%v trace=%v", err, trace)
	}
	proof := map[string]any{"scope": "fixed-sample-research", "factoryTransaction": tx.Hash(), "factory": factory, "locker": locker, "factoryNonce": 0, "lockerCreateNonce": 1, "birthBlock": header.Number, "birthHash": header.Hash(), "trace": trace, "constructorExternalCalls": 0, "factoryRuntimeHash": crypto.Keccak256Hash(actualFactory), "lockerRuntimeHash": crypto.Keccak256Hash(actualLocker)}
	b, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory(), "creation-proof.json"), append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))
}

// TestPoolReceiptRoute discovers the position and custodian from the Initialize receipt.
//
// Version:
//   - 2026-09-23: Added.
func TestPoolReceiptRoute(t *testing.T) {
	r := records(t)
	receipt := decode[types.Receipt](t, r, "pool_receipt")
	header := decode[types.Header](t, r, "pool_header")
	if receipt.Status != 1 || receipt.BlockHash != header.Hash() || receipt.BlockNumber.Cmp(header.Number) != 0 {
		t.Fatal("pool creation receipt identity mismatch")
	}
	initialize := crypto.Keccak256Hash([]byte("Initialize(bytes32,address,address,uint24,int24,address,uint160,int24)"))
	modify := crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)"))
	transfer := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	var initialized, position, minted bool
	for _, l := range receipt.Logs {
		if l.Removed || l.BlockHash != header.Hash() || l.TxHash != receipt.TxHash {
			t.Fatal("noncanonical log")
		}
		if l.Address == core && len(l.Topics) == 4 && l.Topics[0] == initialize && l.Topics[1] == pool {
			if len(l.Data) != 160 || new(big.Int).SetBytes(l.Data[64:96]).Sign() != 0 {
				t.Fatal("hook or initialization mismatch")
			}
			initialized = true
		}
		if l.Address == core && len(l.Topics) == 3 && l.Topics[0] == modify && l.Topics[1] == pool && common.BytesToAddress(l.Topics[2][:]) == manager {
			if len(l.Data) != 128 || new(big.Int).SetBytes(l.Data[96:128]).Cmp(big.NewInt(3073314)) != 0 {
				t.Fatal("unexpected position salt")
			}
			position = true
		}
		if l.Address == manager && len(l.Topics) == 4 && l.Topics[0] == transfer && l.Topics[1] == (common.Hash{}) && common.BytesToAddress(l.Topics[2][:]) == locker && l.Topics[3].Big().Cmp(big.NewInt(3073314)) == 0 {
			minted = true
		}
	}
	if !initialized || !position || !minted {
		t.Fatalf("incomplete discovery: initialized=%t position=%t minted=%t", initialized, position, minted)
	}
}

// TestPermitClosure checks the deployed manager's contract-signature path and selector boundaries.
// The full closure argument additionally depends on the source review in README.md.
//
// Version:
//   - 2026-09-23: Added.
func TestPermitClosure(t *testing.T) {
	r := records(t)
	var mc, lc compiled
	load(t, "PositionManager-compiled.json", &mc)
	load(t, "LaunchLocker-compiled.json", &lc)
	code := common.FromHex(decode[string](t, r, "manager_code"))
	if !matchesRuntime(mc, code) {
		t.Fatal("manager runtime differs from compiled source")
	}
	lABI, err := abi.JSON(bytes.NewReader(lc.Contract.ABI))
	if err != nil {
		t.Fatal(err)
	}
	if lABI.HasFallback() {
		t.Fatal("unexpected fallback")
	}
	for _, method := range lABI.Methods {
		for _, prohibited := range []string{"isValidSignature(bytes32,bytes)", "approve(address,uint256)", "setApprovalForAll(address,bool)", "transferFrom(address,address,uint256)"} {
			if bytes.Equal(method.ID, crypto.Keccak256([]byte(prohibited))[:4]) {
				t.Fatal("unexpected permission selector", method.Sig)
			}
		}
	}
	mABI, err := abi.JSON(bytes.NewReader(mc.Contract.ABI))
	if err != nil {
		t.Fatal(err)
	}
	// Even if a registered quote equals PositionManager, ERC20 payout selectors
	// must not resolve to an NFT authority-changing method.
	for _, method := range mABI.Methods {
		if bytes.Equal(method.ID, crypto.Keccak256([]byte("transfer(address,uint256)"))[:4]) || bytes.Equal(method.ID, crypto.Keccak256([]byte("balanceOf(address)"))[:4]) && method.Sig != "balanceOf(address)" {
			t.Fatal("unexpected payout selector collision", method.Sig)
		}
	}
	for _, n := range []int{0, 64, 65, 96} {
		t.Run(fmt.Sprintf("signature_%d", n), func(t *testing.T) {
			cfg := chainConfig(t)
			cfg.Origin = common.HexToAddress("0x1234")
			cfg.State.SetCode(manager, code, tracing.CodeChangeUnspecified)
			cfg.State.SetCode(locker, common.FromHex(decode[string](t, r, "locker_code")), tracing.CodeChangeUnspecified)
			called1271 := false
			cfg.EVMConfig.Tracer = &tracing.Hooks{OnEnter: func(depth int, typ byte, from, to common.Address, input []byte, gas uint64, value *big.Int) {
				if from == manager && to == locker && typ == byte(vm.STATICCALL) && bytes.HasPrefix(input, crypto.Keccak256([]byte("isValidSignature(bytes32,bytes)"))[:4]) {
					called1271 = true
				}
			}}
			data := encode(t, mc, "permitForAll", locker, cfg.Origin, true, big.NewInt(2000000000), big.NewInt(0), bytes.Repeat([]byte{1}, n))
			if _, _, err := evmruntime.Call(manager, data, cfg); err == nil || !called1271 {
				t.Fatalf("contract permit did not reject through ERC1271: called=%t err=%v", called1271, err)
			}
		})
	}
}

// TestConstructorCounterexample rejects a constructor that approves an operator then returns identical code.
//
// Version:
//   - 2026-09-23: Added.
func TestConstructorCounterexample(t *testing.T) {
	r := records(t)
	var lc, mc compiled
	load(t, "LaunchLocker-compiled.json", &lc)
	load(t, "PositionManager-compiled.json", &mc)
	code := common.FromHex(decode[string](t, r, "locker_code"))
	// Return identical code, but CALL a manager with setApprovalForAll first.
	payload := append(crypto.Keccak256([]byte("setApprovalForAll(address,bool)"))[:4], common.LeftPadBytes(common.HexToAddress("0x1234").Bytes(), 32)...)
	payload = append(payload, common.LeftPadBytes([]byte{1}, 32)...)
	init := []byte{}
	for i := 0; i < len(payload); i += 32 {
		end := min(i+32, len(payload))
		init = append(init, 0x7f)
		init = append(init, common.RightPadBytes(payload[i:end], 32)...)
		init = append(init, 0x60, byte(i), 0x52)
	}
	init = append(init, common.FromHex("0x60006000604460006000")...)
	init = append(init, 0x73)
	init = append(init, manager[:]...)
	init = append(init, common.FromHex("0x5af150")...)
	offset := len(init) + 15
	init = append(init, 0x61, byte(len(code)>>8), byte(len(code)), 0x61, byte(offset>>8), byte(offset), 0x60, 0, 0x39, 0x61, byte(len(code)>>8), byte(len(code)), 0x60, 0, 0xf3)
	init = append(init, code...)
	cfg := chainConfig(t)
	cfg.State.SetCode(manager, common.FromHex(decode[string](t, r, "manager_code")), tracing.CodeChangeUnspecified)
	approval := false
	cfg.EVMConfig.Tracer = &tracing.Hooks{OnEnter: func(depth int, typ byte, from, to common.Address, input []byte, gas uint64, value *big.Int) {
		if to == manager && typ == byte(vm.CALL) && bytes.Equal(input, payload) {
			approval = true
		}
	}}
	runtimeCode, created, _, err := evmruntime.Create(init, cfg)
	if err != nil || !approval || !bytes.Equal(code, runtimeCode) || !matchesRuntime(lc, runtimeCode) || matchesInit(lc, init, common.LeftPadBytes(manager[:], 32)) {
		t.Fatalf("counterexample not rejected by creation identity: approval=%t err=%v", approval, err)
	}
	result, _, err := evmruntime.Call(manager, encode(t, mc, "isApprovedForAll", created, common.HexToAddress("0x1234")), cfg)
	if err != nil || new(big.Int).SetBytes(result).Cmp(big.NewInt(1)) != 0 {
		t.Fatal("counterexample did not persist operator approval", err)
	}
}

// TestCollectRemovesZeroPrincipal traces the actual locker runtime with an injected manager.
//
// Version:
//   - 2026-09-23: Added.
func TestCollectRemovesZeroPrincipal(t *testing.T) {
	r := records(t)
	var lc, mc compiled
	load(t, "LaunchLocker-compiled.json", &lc)
	load(t, "PositionManager-compiled.json", &mc)
	cfg := chainConfig(t)
	cfg.Origin = factory
	cfg.State.SetCode(locker, common.FromHex(decode[string](t, r, "locker_code")), tracing.CodeChangeUnspecified)
	// ownerOf fixture: all NFTs below are owned by the locker.
	getter := append([]byte{0x7f}, common.LeftPadBytes(locker[:], 32)...)
	getter = append(getter, common.FromHex("0x60005260206000f3")...)
	cfg.State.SetCode(manager, getter, tracing.CodeChangeUnspecified)
	token := common.HexToAddress("0x3ee1a9806f4bf62bdd753b9907878f7f85c637f7")
	recipients := []struct {
		Payout common.Address
		Bps    uint16
	}{{common.HexToAddress("0xdead"), 10000}}
	registration := encode(t, lc, "register", big.NewInt(3073314), token, common.Address{}, recipients)
	if _, _, err := evmruntime.Call(locker, registration, cfg); err != nil {
		t.Fatal(err)
	}
	// No fees owed by this fixture; inspect the manager call instead of moving funds.
	cfg.State.SetCode(manager, []byte{0x00}, tracing.CodeChangeUnspecified)
	cfg.State.SetCode(token, common.FromHex("0x600060005260206000f3"), tracing.CodeChangeUnspecified)
	cfg.Origin = common.HexToAddress("0x1234")
	ma, err := abi.JSON(bytes.NewReader(mc.Contract.ABI))
	if err != nil {
		t.Fatal(err)
	}
	bytesType, err := abi.NewType("bytes", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	arrayType, err := abi.NewType("bytes[]", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	cfg.EVMConfig.Tracer = &tracing.Hooks{OnEnter: func(depth int, typ byte, from, to common.Address, input []byte, gas uint64, value *big.Int) {
		if from != locker || to != manager {
			return
		}
		called = true
		m := ma.Methods["modifyLiquidities"]
		if !bytes.HasPrefix(input, m.ID) {
			t.Fatal("unexpected manager operation")
		}
		args, err := m.Inputs.Unpack(input[4:])
		if err != nil {
			t.Fatal(err)
		}
		inner, err := (abi.Arguments{{Type: bytesType}, {Type: arrayType}}).Unpack(args[0].([]byte))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(inner[0].([]byte), []byte{1, 17}) {
			t.Fatal("actions are not DECREASE_LIQUIDITY then TAKE_PAIR")
		}
		p := inner[1].([][]byte)
		if len(p) != 2 || len(p[0]) < 160 || new(big.Int).SetBytes(p[0][32:64]).Sign() != 0 || new(big.Int).SetBytes(p[0][:32]).Cmp(big.NewInt(3073314)) != 0 {
			t.Fatal("principal decrease is nonzero or wrong NFT")
		}
	}}
	if _, _, err := evmruntime.Call(locker, encode(t, lc, "collect", big.NewInt(3073314)), cfg); err != nil || !called {
		t.Fatalf("fee collection failed: called=%t err=%v", called, err)
	}
}

func aggregateResults(t *testing.T, r map[string]record, name string) [][]byte {
	t.Helper()
	a, err := abi.JSON(strings.NewReader(`[{"type":"function","name":"aggregate3","inputs":[],"outputs":[{"name":"results","type":"tuple[]","components":[{"name":"success","type":"bool"},{"name":"returnData","type":"bytes"}]}]}]`))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := a.Unpack("aggregate3", common.FromHex(decode[string](t, r, name)))
	if err != nil {
		t.Fatal(err)
	}
	type item struct {
		Success    bool
		ReturnData []byte
	}
	values := *abi.ConvertType(decoded[0], new([]item)).(*[]item)
	var result [][]byte
	for _, value := range values {
		if !value.Success {
			t.Fatal("nested read failed", name)
		}
		result = append(result, value.ReturnData)
	}
	return result
}

func signedWord(b []byte) *big.Int {
	n := new(big.Int).SetBytes(b)
	if b[0]&0x80 != 0 {
		n.Sub(n, new(big.Int).Lsh(big.NewInt(1), uint(8*len(b))))
	}
	return n
}

// TestCurrentPrincipalCoverage verifies all ticks, including the inactive position, and zero denominators.
//
// Version:
//   - 2026-09-23: Added.
func TestCurrentPrincipalCoverage(t *testing.T) {
	r := records(t)
	h := decode[types.Header](t, r, "observation_header")
	if h.Number.Uint64() != 51604306 || h.Hash() != common.HexToHash("0xeaa77d3a8cce998496372ea11342eff355c61f8ea86a5def29adc7633b6e62fe") {
		t.Fatal("wrong observation")
	}
	for _, name := range []string{"state", "bitmap", "ticks", "factory_code", "locker_code", "manager_code"} {
		var q []json.RawMessage
		if err := json.Unmarshal(r[name].Params, &q); err != nil || len(q) != 2 {
			t.Fatal("invalid state query", name, err)
		}
		var anchor struct {
			BlockHash        common.Hash
			RequireCanonical bool
		}
		if err := json.Unmarshal(q[1], &anchor); err != nil || anchor.BlockHash != h.Hash() || !anchor.RequireCanonical {
			t.Fatal("unpinned query", name, err)
		}
	}
	s := aggregateResults(t, r, "state")
	if len(s) != 11 || len(s[0]) != 192 || crypto.Keccak256Hash(s[0][:160]) != pool || new(big.Int).SetBytes(s[0][128:160]).Sign() != 0 || new(big.Int).SetBytes(s[0][96:128]).Int64() != 200 {
		t.Fatal("pool key, hook, or spacing mismatch")
	}
	if common.BytesToAddress(s[2]) != locker || new(big.Int).SetBytes(s[3]).Sign() != 0 || common.BytesToAddress(s[7]) != manager || common.BytesToAddress(s[8]) != factory || common.BytesToAddress(s[9]) != core || common.BytesToAddress(s[10]) != core {
		t.Fatal("custodian, approval, or deployment bindings differ")
	}
	packed := new(big.Int).SetBytes(s[0][160:192])
	mask := big.NewInt(0xffffff)
	lower := int32(new(big.Int).And(new(big.Int).Rsh(new(big.Int).Set(packed), 8), mask).Int64())
	upper := int32(new(big.Int).And(new(big.Int).Rsh(new(big.Int).Set(packed), 32), mask).Int64())
	if lower&(1<<23) != 0 {
		lower -= 1 << 24
	}
	if upper&(1<<23) != 0 {
		upper -= 1 << 24
	}
	liquidity := new(big.Int).SetBytes(s[1])
	if len(s[4]) != 96 || liquidity.Sign() <= 0 || liquidity.Cmp(new(big.Int).SetBytes(s[4][:32])) != 0 || lower != -887200 || upper != 184400 {
		t.Fatal("NFT and core position differ")
	}
	bitmap := aggregateResults(t, r, "bitmap")
	if len(bitmap) != 36 {
		t.Fatal("incomplete full-range bitmap")
	}
	var indices []int32
	for i, b := range bitmap {
		if len(b) != 32 {
			t.Fatal("invalid bitmap")
		}
		n := new(big.Int).SetBytes(b)
		for bit := 0; bit < 256; bit++ {
			if n.Bit(bit) != 0 {
				indices = append(indices, int32(((i-18)*256+bit)*200))
			}
		}
	}
	if len(indices) != 2 || indices[0] != lower || indices[1] != upper {
		t.Fatal("additional or missing initialized ticks")
	}
	ticks := aggregateResults(t, r, "ticks")
	if len(ticks) != 2 || len(s[5]) != 128 || len(s[6]) != 32 {
		t.Fatal("invalid state response")
	}
	state := clliquidity.State{SqrtPriceX96: new(big.Int).SetBytes(s[5][:32]), Tick: int32(signedWord(s[5][32:64]).Int64()), Spacing: 200, ActiveLiquidity: new(big.Int).SetBytes(s[6]), Complete: true}
	for i, tick := range ticks {
		if len(tick) != 64 {
			t.Fatal("invalid tick")
		}
		gross, net := new(big.Int).SetBytes(tick[:32]), signedWord(tick[32:64])
		expectedNet := new(big.Int).Set(liquidity)
		if i == 1 {
			expectedNet.Neg(expectedNet)
		}
		if gross.Cmp(liquidity) != 0 || net.Cmp(expectedNet) != 0 {
			t.Fatal("uncovered position principal")
		}
		state.Ticks = append(state.Ticks, clliquidity.Tick{Index: indices[i], Gross: gross, Net: net})
	}
	amounts, err := clliquidity.Calculate(state)
	if err != nil {
		t.Fatal(err)
	}
	a0, a1, err := clliquidity.PositionPrincipal(state.SqrtPriceX96, liquidity, lower, upper)
	if err != nil || a0.Sign() != 0 || a1.Sign() <= 0 || amounts.Token0.Sign() != 0 || amounts.Token1.Cmp(new(big.Int).Quo(a1.Num(), a1.Denom())) != 0 {
		t.Fatal("principal mismatch", err)
	}
	proof := map[string]any{"poolId": pool, "block": h.Number, "blockHash": h.Hash(), "tokenId": 3073314, "positions": 1, "bitmapWords": 36, "ticks": 2, "allGrossAndNetCovered": true, "principalBaseUnits0": amounts.Token0.String(), "principalBaseUnits1": amounts.Token1.String(), "permanentProtectionPercentage0": nil, "permanentProtectionPercentage1": "100", "allPositionsProtected": true, "requiresCreationAndAuthorityProof": true, "classification": "research-only-limited-model", "currentTick": state.Tick, "activeLiquidity": state.ActiveLiquidity.String()}
	b, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory(), "principal-proof.json"), append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))
}
