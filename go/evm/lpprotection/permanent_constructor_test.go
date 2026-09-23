//go:build lp_v4_model

package lpprotection

import (
	"bytes"
	"math/big"
	"strings"
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

func permanentVM(t *testing.T, origin common.Address, header *types.Header) *evmruntime.Config {
	t.Helper()
	db, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	chain := *params.AllDevChainProtocolChanges
	chain.ChainID = big.NewInt(8453)
	return &evmruntime.Config{State: db, ChainConfig: &chain, Origin: origin, GasLimit: 20000000, BlockNumber: header.Number, Time: header.Time}
}

// TestPermanentConstructorReplay reproduces the reviewed constructor and excludes external approval calls.
// This is an offline model regression test; production does not run an EVM.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentConstructorReplay(t *testing.T) {
	f, req := permanentSample(t)
	hint := req.CreationHints[0]
	tx := f.data.Transactions[hint.TransactionHash]
	origin, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8453)), tx)
	if err != nil {
		t.Fatal(err)
	}
	factory, locker := hint.Factory, crypto.CreateAddress(hint.Factory, 1)
	cfg := permanentVM(t, origin, f.data.Headers[50940130])
	creates := 0
	cfg.EVMConfig.Tracer = &tracing.Hooks{OnEnter: func(depth int, op byte, from, to common.Address, input []byte, _ uint64, _ *big.Int) {
		if op != byte(vm.CREATE) || depth == 0 && (from != origin || to != factory || !bytes.Equal(input, tx.Data())) || depth == 1 && (from != factory || to != locker) || depth > 1 {
			t.Fatalf("unexpected constructor interaction: depth=%d opcode=%d from=%s to=%s", depth, op, from, to)
		}
		creates++
	}}
	code, address, _, err := evmruntime.Create(tx.Data(), cfg)
	expectedFactory := common.FromHex(f.data.Codes[strings.ToLower(factory.Hex()+req.Principal.Snapshot.BlockHash.Hex())])
	expectedLocker := common.FromHex(f.data.Codes[strings.ToLower(locker.Hex()+req.Principal.Snapshot.BlockHash.Hex())])
	if err != nil || address != factory || creates != 2 || !bytes.Equal(code, expectedFactory) || !bytes.Equal(cfg.State.GetCode(locker), expectedLocker) {
		t.Fatalf("constructor replay mismatch: created=%s calls=%d error=%v", address, creates, err)
	}
}

// TestPermanentFakeConstructor creates an approval before returning reviewed runtime, then verifies rejection.
//
// Version:
//   - 2026-09-24: Added.
func TestPermanentFakeConstructor(t *testing.T) {
	f, req := permanentSample(t)
	manager := common.HexToAddress("0x7c5f5a4bbd8fd63184577525326123b519429bdc")
	operator := common.HexToAddress("0x123456")
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	origin := crypto.PubkeyToAddress(key.PublicKey)
	factory := crypto.CreateAddress(origin, 0)
	locker := crypto.CreateAddress(factory, 1)
	code := runtime(t, "v4LaunchLocker")
	factoryCode := runtime(t, "v4LaunchFactory")
	for _, offset := range template("v4LaunchLocker").fields["factory"] {
		copy(code[offset:offset+32], common.LeftPadBytes(factory[:], 32))
	}
	for _, offset := range template("v4LaunchFactory").fields["locker"] {
		copy(factoryCode[offset:offset+32], common.LeftPadBytes(locker[:], 32))
	}
	payload := query("setApprovalForAll(address,bool)", operator.Big(), big.NewInt(1))
	// Hand-assemble a malicious constructor that grants an operator and returns
	// exactly the reviewed runtime. There is deliberately no Solidity dependency.
	var init []byte
	for i := 0; i < len(payload); i += 32 {
		init = append(init, 0x7f)
		init = append(init, common.RightPadBytes(payload[i:min(i+32, len(payload))], 32)...)
		init = append(init, 0x60, byte(i), 0x52)
	}
	init = append(init, common.FromHex("0x60006000604460006000")...)
	init = append(init, 0x73)
	init = append(init, manager[:]...)
	init = append(init, common.FromHex("0x5af150")...)
	offset := len(init) + 15
	init = append(init, 0x61, byte(len(code)>>8), byte(len(code)), 0x61, byte(offset>>8), byte(offset), 0x60, 0, 0x39, 0x61, byte(len(code)>>8), byte(len(code)), 0x60, 0, 0xf3)
	init = append(init, code...)
	// The malicious factory creates that child at nonce 1, then returns the
	// matching factory runtime. Both immutable address relationships still hold.
	child := init
	const childOffset = 33 // 18-byte CREATE prefix and 15-byte runtime return.
	factoryOffset := childOffset + len(child)
	init = []byte{0x61, byte(len(child) >> 8), byte(len(child)), 0x61, 0, childOffset, 0x60, 0, 0x39, 0x61, byte(len(child) >> 8), byte(len(child)), 0x60, 0, 0x60, 0, 0xf0, 0x50}
	init = append(init, 0x61, byte(len(factoryCode)>>8), byte(len(factoryCode)), 0x61, byte(factoryOffset>>8), byte(factoryOffset), 0x60, 0, 0x39, 0x61, byte(len(factoryCode)>>8), byte(len(factoryCode)), 0x60, 0, 0xf3)
	init = append(init, child...)
	init = append(init, factoryCode...)
	cfg := permanentVM(t, origin, f.data.Headers[50940130])
	cfg.State.SetCode(manager, runtime(t, "v4_position_manager"), tracing.CodeChangeUnspecified)
	runtimeCode, created, _, err := evmruntime.Create(init, cfg)
	if err != nil || created != factory || !bytes.Equal(runtimeCode, factoryCode) || !bytes.Equal(cfg.State.GetCode(locker), code) {
		t.Fatal("counterexample did not return reviewed code", err)
	}
	if _, ok := match(runtimeCode, "v4LaunchFactory"); !ok {
		t.Fatal("counterexample must pass runtime-only matching")
	}
	if _, ok := match(cfg.State.GetCode(locker), "v4LaunchLocker"); !ok {
		t.Fatal("counterexample must pass locker runtime matching")
	}
	v, _, err := evmruntime.Call(manager, query("isApprovedForAll(address,address)", locker.Big(), operator.Big()), cfg)
	if err != nil || new(big.Int).SetBytes(v).Cmp(big.NewInt(1)) != 0 {
		t.Fatal("counterexample approval missing", err)
	}
	tx, err := types.SignTx(types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(8453), Gas: 20000000, GasFeeCap: big.NewInt(1), GasTipCap: new(big.Int), Value: new(big.Int), Data: init}), types.LatestSignerForChainID(big.NewInt(8453)), key)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := launchFactoryDeployment(tx, created, req.Principal.Pool, manager, common.HexToAddress("0x000000000022d473030f116ddee9f6b43ac78ba3")); ok {
		t.Fatal("constructor with approval accepted")
	}
}
