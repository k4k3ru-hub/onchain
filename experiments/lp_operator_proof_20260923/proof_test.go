//go:build lp_operator_research

package operatorproof

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	evmruntime "github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

type record struct {
	Method string
	Params json.RawMessage
	Result json.RawMessage
}

func directory() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func load(t *testing.T, file string, dst any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(directory(), file))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatal(err)
	}
}

func decode[T any](t *testing.T, records map[string]record, name string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(records[name].Result, &value); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return value
}

func hexNumber(t *testing.T, value string) uint64 {
	t.Helper()
	n, ok := new(big.Int).SetString(value[2:], 16)
	if !ok || !n.IsUint64() {
		t.Fatal("invalid number")
	}
	return n.Uint64()
}

// executableOpcodes skips PUSH payloads and the Solidity metadata trailer.
func executableOpcodes(t *testing.T, code []byte, metadata bool) map[byte]bool {
	t.Helper()
	if metadata {
		n := int(code[len(code)-2])<<8 | int(code[len(code)-1])
		if n+2 >= len(code) {
			t.Fatal("invalid metadata trailer")
		}
		code = code[:len(code)-n-2]
	}
	found := map[byte]bool{}
	for i := 0; i < len(code); i++ {
		op := code[i]
		found[op] = true
		if op >= 0x60 && op <= 0x7f {
			i += int(op - 0x5f)
		}
	}
	return found
}

// TestCreationProof verifies archived creation input against independent compilation,
// replays the observed CreateX code locally, and establishes the clone's birth range.
// This is an isolated proof for one fixed sample, not a production classifier.
//
// Version:
//   - 2026-09-23: Added.
func TestCreationProof(t *testing.T) {
	var evidence struct{ Results map[string]record }
	load(t, "rpc-evidence.json", &evidence)
	r := evidence.Results
	if decode[string](t, r, "chain") != "0x2105" {
		t.Fatal("wrong chain")
	}
	factory := common.HexToAddress("0x932d0b4c00a2a33ef1ec5fe0aa981bd1a00a6f5c")
	manager := common.HexToAddress("0xe1f8cd9ac4e4a65f54f38a5cdafca44f6dd68b53")
	locker := common.HexToAddress("0xdbf7a3d301e39871e6644a17fb1af1a3889078af")
	createx := common.HexToAddress("0xba5ed099633d3b313e4d5f7bdc1305d3c28ba5ed")
	proxy := common.HexToAddress("0x6b9d4aaccc4d8b924a86c07b3b7129ab1b922ca1")
	tx := decode[types.Transaction](t, r, "factory_tx")
	fr := decode[types.Receipt](t, r, "factory_receipt")
	fh := decode[types.Header](t, r, "factory_header")
	pr := decode[types.Receipt](t, r, "pool_receipt")
	ph := decode[types.Header](t, r, "pool_header")
	th := decode[types.Header](t, r, "observation_header")
	for _, name := range []string{"factory_header", "pool_header", "observation_header"} {
		before, after := decode[types.Header](t, r, name), decode[types.Header](t, r, name+"_final")
		if before.Hash() != after.Hash() {
			t.Fatal("canonical block changed", name)
		}
	}
	fprev, pprev := decode[types.Header](t, r, "factory_prior_header"), decode[types.Header](t, r, "pool_prior_header")
	if fprev.Hash() != fh.ParentHash || pprev.Hash() != ph.ParentHash {
		t.Fatal("prior block mismatch")
	}
	if fr.Status != 1 || tx.Hash() != fr.TxHash || fr.BlockHash != fh.Hash() || fr.BlockNumber.Cmp(fh.Number) != 0 || pr.Status != 1 || pr.BlockHash != ph.Hash() || ph.Number.Cmp(pr.BlockNumber) != 0 || th.Number.Uint64() != 51596896 || th.Hash() != common.HexToHash("0xacc268320c2198258d4b13b4fb4f90bb810d626b9329fe58ac60e52c55796ba2") {
		t.Fatal("receipt or header identity mismatch")
	}
	origin, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8453)), &tx)
	if err != nil || tx.To() == nil || *tx.To() != createx || tx.Value().Sign() != 0 {
		t.Fatal("invalid creation transaction", err)
	}
	data := tx.Data()
	if !bytes.Equal(data[:4], crypto.Keccak256([]byte("deployCreate3(bytes32,bytes)"))[:4]) {
		t.Fatal("unexpected creation method")
	}
	offset := new(big.Int).SetBytes(data[36:68]).Uint64() + 4
	length := new(big.Int).SetBytes(data[offset : offset+32]).Uint64()
	init := data[offset+32 : offset+32+length]
	var compiled struct {
		Compiler string
		Contract struct {
			EVM struct {
				Bytecode         struct{ Object string }
				DeployedBytecode struct {
					Object              string
					ImmutableReferences map[string][]struct{ Start, Length int }
				}
			}
		}
	}
	load(t, "factory-compiled.json", &compiled)
	creationCode := common.FromHex(compiled.Contract.EVM.Bytecode.Object)
	if len(init) != len(creationCode)+160 || !bytes.Equal(init[:len(creationCode)], creationCode) {
		t.Fatal("creation input does not match independently compiled source")
	}
	actualFactory := common.FromHex(decode[string](t, r, "factory_creation_code"))
	if decode[string](t, r, "factory_creation_code") != decode[string](t, r, "factory_pool_creation_code") {
		t.Fatal("factory runtime changed")
	}
	normalized := bytes.Clone(actualFactory)
	for _, sites := range compiled.Contract.EVM.DeployedBytecode.ImmutableReferences {
		var value []byte
		for _, site := range sites {
			current := actualFactory[site.Start : site.Start+site.Length]
			if value != nil && !bytes.Equal(value, current) {
				t.Fatal("inconsistent immutable")
			}
			value = current
			clear(normalized[site.Start : site.Start+site.Length])
		}
	}
	if !bytes.Equal(normalized, common.FromHex(compiled.Contract.EVM.DeployedBytecode.Object)) {
		t.Fatal("factory runtime mismatch")
	}
	createxCode := common.FromHex(decode[string](t, r, "createx_code"))
	if decode[string](t, r, "createx_code") != decode[string](t, r, "createx_prior_code") {
		t.Fatal("CreateX was not present before the creation block")
	}
	for name, code := range map[string][]byte{"CreateX": createxCode, "Factory": actualFactory} {
		ops := executableOpcodes(t, code, true)
		if ops[byte(vm.SELFDESTRUCT)] || ops[byte(vm.DELEGATECALL)] || ops[byte(vm.CALLCODE)] || name == "CreateX" && (ops[byte(vm.SLOAD)] || ops[byte(vm.SSTORE)]) {
			t.Fatal("unsupported mutable deployment path", name)
		}
	}
	s, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	s.CreateAccount(createx)
	s.SetCode(createx, createxCode, tracing.CodeChangeUnspecified)
	// Only the canonical manager's factory() getter is needed by this constructor.
	// Substitute its actual pinned RPC response; unexpected calls fail below.
	getter := common.FromHex(decode[string](t, r, "manager_factory_at_creation"))
	if len(getter) != 32 || common.BytesToAddress(getter) != common.HexToAddress("0xf8f2eb4940cfe7d13603dddd87f123820fc061ef") {
		t.Fatal("unexpected canonical pool factory")
	}
	s.CreateAccount(manager)
	s.SetCode(manager, append(append([]byte{0x7f}, getter...), common.FromHex("0x60005260206000f3")...), tracing.CodeChangeUnspecified)
	proxyInit := common.FromHex("0x67363d3d37363d34f03d5260086018f3")
	var steps []string
	hooks := &tracing.Hooks{OnEnter: func(depth int, typ byte, from, to common.Address, input []byte, gas uint64, value *big.Int) {
		steps = append(steps, fmt.Sprintf("%d %s %s -> %s", depth, vm.OpCode(typ), from.Hex(), to.Hex()))
		switch {
		case depth == 0 && typ == byte(vm.CALL) && from == origin && to == createx:
		case typ == byte(vm.CREATE2) && from == createx && to == proxy && bytes.Equal(input, proxyInit):
		case typ == byte(vm.CALL) && from == createx && to == proxy && bytes.Equal(input, init):
		case typ == byte(vm.CREATE) && from == proxy && to == factory && bytes.Equal(input, init):
		case typ == byte(vm.STATICCALL) && from == factory && to == manager && bytes.Equal(input, crypto.Keccak256([]byte("factory()"))[:4]):
		default:
			t.Fatalf("unexpected replay path: %s", steps[len(steps)-1])
		}
	}}
	chain := *params.AllDevChainProtocolChanges
	chain.ChainID = big.NewInt(8453)
	cfg := &evmruntime.Config{State: s, Origin: origin, ChainConfig: &chain, BlockNumber: fh.Number, Time: fh.Time, GasLimit: 16000000, EVMConfig: vm.Config{Tracer: hooks}}
	_, _, err = evmruntime.Call(createx, data, cfg)
	if err != nil || len(steps) != 5 || !bytes.Equal(s.GetCode(factory), actualFactory) || !bytes.Equal(s.GetCode(proxy), common.FromHex(decode[string](t, r, "create3_proxy_code"))) || s.GetNonce(proxy) != 2 {
		t.Fatalf("deployment replay differs: steps=%v err=%v", steps, err)
	}
	// This eight-byte proxy has no path to reset its nonce or delete itself.
	if !bytes.Equal(s.GetCode(proxy), common.FromHex("0x363d3d37363d34f0")) || crypto.CreateAddress(proxy, 1) != factory {
		t.Fatal("factory creation address is not bound to unique proxy nonce")
	}
	cfg.EVMConfig.Tracer = nil
	if _, _, againErr := evmruntime.Call(createx, data, cfg); againErr == nil || s.GetNonce(proxy) != 2 || !bytes.Equal(s.GetCode(factory), actualFactory) {
		t.Fatal("the same creation salt unexpectedly recreated the factory")
	}
	var creation *types.Log
	topic := crypto.Keccak256Hash([]byte("LockCreated(address,address,uint256,uint32,address,uint16,uint16)"))
	for _, l := range pr.Logs {
		if l.Address == factory && len(l.Topics) == 3 && l.Topics[0] == topic && common.BytesToAddress(l.Topics[2][:]) == locker {
			if creation != nil || l.Removed || l.BlockHash != ph.Hash() || l.TxHash != pr.TxHash || len(l.Data) != 160 {
				t.Fatal("invalid or duplicate locker creation")
			}
			creation = l
		}
	}
	if creation == nil || new(big.Int).SetBytes(creation.Data[:32]).Uint64() != 6628901 {
		t.Fatal("locker creation not associated with the sample NFT")
	}
	before := hexNumber(t, decode[string](t, r, "factory_nonce_before_pool"))
	after := hexNumber(t, decode[string](t, r, "factory_nonce_at_pool"))
	if before >= after || after-before > 1024 {
		t.Fatal("unbounded or empty nonce range")
	}
	var nonce uint64
	for n := before; n < after; n++ {
		if crypto.CreateAddress(factory, n) == locker {
			nonce = n
		}
	}
	if nonce == 0 {
		t.Fatal("locker is not a fresh CREATE address in the creation block")
	}
	clone := common.FromHex(decode[string](t, r, "locker_creation_code"))
	expected := common.FromHex("0x363d3d373d3d3d363d73fe678bffc3c1c8d1de4478cc5c3e1b93ee4638ae5af43d82803e903d91602b57fd5bf3")
	if !bytes.Equal(clone, expected) {
		t.Fatal("unexpected clone runtime")
	}
	// The receipt identity and first-ever CREATE nonce establish the start block.
	// Enforce a contiguous scan, including that block and all events within it.
	logCount := 0
	for from := ph.Number.Uint64(); from <= th.Number.Uint64(); from += 1000 {
		to := min(from+999, th.Number.Uint64())
		name := fmt.Sprintf("operators_%d_%d", from, to)
		var q []struct {
			Address            common.Address
			FromBlock, ToBlock string
			Topics             []common.Hash
		}
		if err := json.Unmarshal(r[name].Params, &q); err != nil || len(q) != 1 || q[0].Address != manager || q[0].FromBlock != fmt.Sprintf("0x%x", from) || q[0].ToBlock != fmt.Sprintf("0x%x", to) || len(q[0].Topics) != 2 || q[0].Topics[0] != crypto.Keccak256Hash([]byte("ApprovalForAll(address,address,bool)")) || q[0].Topics[1] != common.BytesToHash(locker[:]) {
			t.Fatal("approval query coverage mismatch", name, err)
		}
		logs := decode[[]types.Log](t, r, name)
		logCount += len(logs)
	}
	if logCount != 0 {
		t.Fatal("operators discovered: current state must be checked before a verdict")
	}
	proof := map[string]any{"scope": "research-only-single-sample", "chainId": 8453, "factory": factory, "locker": locker, "manager": manager, "firstCustodianBlock": ph.Number.Uint64(), "firstCustodianBlockHash": ph.Hash(), "observationBlock": th.Number.Uint64(), "observationHash": th.Hash(), "factoryCreationTx": tx.Hash(), "factoryConstructorMatches": true, "factoryRuntimeMatches": true, "createXReplaySteps": steps, "createXCodeHash": crypto.Keccak256Hash(createxCode), "uniqueProxyNonce": 1, "factoryNonceBefore": before, "factoryNonceAfter": after, "lockerCreateNonce": nonce, "approvalLogRanges": 7, "approvalLogCount": logCount, "operatorSetEmpty": true, "compiler": compiled.Compiler}
	encoded, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory(), "proof.json"), append(encoded, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
}
